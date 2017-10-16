package main

import (
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	"net/http"
	"strconv"
	"text/template"
	"path"
	"bytes"
)

const (
	DEVICES       = "devices"
	ACCESS_DEVICE = "accessDevice"
	VLAN          = "vlan"
	FLOWS         = "flows"
	KEY_APP_ID    = "appId"
	NETCFG_URL    = "%s/onos/v1/network/configuration"
	FLOWS_URL     = "%s/onos/v1/flows/%s"
	APP_ID        = "com.ciena"
)

type FlowWorker struct{}

type RuleData struct {
	AppId  string
	DPID   string
	VlanId string
}

func (app *Application) Synchronize() {

	// Fetch network config to get access to the list of access device VLAN IDs
	resp, err := http.Get(fmt.Sprintf(NETCFG_URL, app.OnosConnectUrl))
	if err != nil {
		log.Warnf("Unable to read ONOS network configuration : %s", err)
		return
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	var raw map[string]interface{}
	decoder.Decode(&raw)
	netcfg, err := gabs.Consume(raw)
	if err != nil {
		log.Warnf("Unable to consume JSON : %s", err)
		return
	}

	/*
	 * Walk the device list looking for the VLAN values associated with the access
	 * device section of the device. Rules for these values will need to be applied
	 * to the OVS switch
	 */
	need := make(map[string]bool)
	devices, _ := netcfg.Path(DEVICES).ChildrenMap()
	for _, device := range devices {
		if device.Exists(ACCESS_DEVICE, VLAN) {
			vlan := device.Search(ACCESS_DEVICE, VLAN).Data()

			if val, ok := vlan.(string); ok {
				need[val] = false
			} else if _, ok := vlan.(float64); ok {
				val := strconv.Itoa(int(vlan.(float64)))
				need[val] = false
			}
		}
	}

	keys := make([]string, len(need))
	for key, _ := range need {
		keys = append(keys, key)
	}
	log.Debugf("Need rules for VLANs %v", keys)

	// Fetch the current rules on the switch
	resp, err = http.Get(fmt.Sprintf(FLOWS_URL, app.OnosConnectUrl, app.OvsDpid))
	if err != nil {
		log.Warnf("Unable to read ONOS flows for swtich %s: %s", app.OvsDpid, err)
		return
	}
	defer resp.Body.Close()
	decoder = json.NewDecoder(resp.Body)
	decoder.Decode(&raw)
	outer, _ := gabs.Consume(raw)
	flows, _ := outer.Path(FLOWS).Children()

	/*
	 * Iterate over all the flows, only paying attention to those that we created
	 * If there is a flow for a VLAN ID that we know longer care about, i.e. it is
	 * not needed, then delete it. If there is a flow for a VLAN we do care about
	 * mark it as already installed.
	 */
	for _, flow := range flows {
		if flow.Exists(KEY_APP_ID) {
			if APP_ID != flow.Path(KEY_APP_ID).Data().(string) {
				continue
			}

			criterias, _ := flow.Path("selector.criteria").Children()
			for _, criteria := range criterias {
				if criteria.Path("type").Data().(string) != "VLAN_VID" {
					continue
				}
				vlan := strconv.Itoa(int(criteria.Path("vlanId").Data().(float64)))
				if _, ok := need[vlan]; ok {
					// Need this rule, mark as needed and found
					need[vlan] = true
				} else {
					// Rule is not needed, delete it
					log.Infof("[DELETE]: VLAN %s rule", flow.Path("id"))
				}
			}
		}
	}

	// Iterate over all the required VLANs and if we don't have a rule for them
	// then add them
	rule := template.New(path.Base(app.CreateFlowTemplate))
	_, err = rule.ParseFiles(app.CreateFlowTemplate)
	if err != nil {
		log.Warnf("Unable to parse rule creation template '%s' : %s", app.CreateFlowTemplate, err)
		return
	}
	for vlan, have := range need {
		if have {
			log.Debugf("[EXISTS] VLAN %s rule", vlan)
			continue
		}
		log.Infof("[CREATE] VLAN %s rule", vlan)
		data := RuleData{
			AppId:  APP_ID,
			DPID:   app.OvsDpid,
			VlanId: vlan,
		}

		// Create a POST to ONOS
		buf := bytes.NewBuffer(nil)
		err := rule.Execute(buf, &data)
		if err != nil {
			log.Warnf("Unable to execute create rule template: %s", err)
		}
                resp, err := http.Post(fmt.Sprintf(FLOWS_URL, app.OnosConnectUrl, app.OvsDpid), "application/json", buf)
		if err != nil {
			log.Errorf("Error while POSTing rule add to ONOS : %s", err)
			continue
		}
		if int(resp.StatusCode / 100) != 2 {
			log.Errorf("Error response code while POSTing rule to ONOS : %s", resp.Status)
			continue
		}
	}
}
