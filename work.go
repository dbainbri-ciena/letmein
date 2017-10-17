package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	"net/http"
	"path"
	"strconv"
	"text/template"
)

const (
	DEVICES         = "devices"
	ACCESS_DEVICE   = "accessDevice"
	VLAN            = "vlan"
	FLOWS           = "flows"
	KEY_APP_ID      = "appId"
	NETCFG_URL      = "%s/onos/v1/network/configuration"
	DEVICES_URL     = "%s/onos/v1/devices"
	PORTS_URL       = "%s/onos/v1/devices/%s/ports"
	FLOWS_URL       = "%s/onos/v1/flows/%s"
	DELETE_FLOW_URL = "%s/onos/v1/flows/%s/%s"
	APP_ID          = "com.ciena"
	DISCOVER        = ":discover"
)

type FlowWorker struct{}

type RuleData struct {
	AppId  string
	DPID   string
	VlanId string
	InPort string
}

func (app *Application) Synchronize() {

	/*
	 * If the DPID is set to ":discover" then attempt to use hueristics to determine the device
	 * in ONOS to use. The hueristic is simple, if the "hw" is "Open vSwtich", the "driver"
	 * is "ovs", and it is "available" then it is our switch. The first match is taken.
	 */
	dpid := app.OvsDpid
	if dpid == DISCOVER {
		resp, err := http.Get(fmt.Sprintf(DEVICES_URL, app.OnosConnectUrl))
		if err != nil {
			log.Errorf("Unable to discover OVS switch to configure : %s", err)
			return
		}
		defer resp.Body.Close()
		if int(resp.StatusCode/100) != 2 {
			log.Errorf("Error response code whilst querying for devices to discover OVS switch : %s",
				resp.Status)
			return
		}
		decoder := json.NewDecoder(resp.Body)
		var raw map[string]interface{}
		err = decoder.Decode(&raw)
		if err != nil {
			log.Errorf("Unable to decode devices response from ONOS : %s", err)
			return
		}
		wrapper, err := gabs.Consume(raw)
		if err != nil {
			log.Errorf("Unable to consume devices JSON object : %s", err)
			return
		}

		devices, err := wrapper.Path("devices").Children()
		if err != nil {
			log.Errorf("Unable to query list of devices from ONOS : %s", err)
			return
		}
		for _, device := range devices {
			if device.Path("hw").Data().(string) == "Open vSwitch" &&
				device.Path("driver").Data().(string) == "ovs" && device.Path("available").Data().(bool) {
				dpid = device.Path("id").Data().(string)
			}
		}
		if dpid == DISCOVER {
			// Unable to discover OVS switch
			log.Error("Unable to discover OVS switch from ONOS, please specify DPID")
			return
		}
	}

	/*
	 * If the PORT is set to ":discover" then attempt to use a hueristic to determine the PORT
	 * of the switch to use. The hueristic is simple, the first non-"local" and "enabled" port
	 * will be used.
	 */
	inPort := app.OvsPort
	if inPort == DISCOVER {
		resp, err := http.Get(fmt.Sprintf(PORTS_URL, app.OnosConnectUrl, dpid))
		if err != nil {
			log.Errorf("Unable to discover OVS switch ports for switch %s : %s", dpid, err)
			return
		}
		defer resp.Body.Close()
		if int(resp.StatusCode/100) != 2 {
			log.Errorf("Error response whilst querying for device ports from switch %s : %s", dpid, err)
			return
		}
		decoder := json.NewDecoder(resp.Body)
		var raw map[string]interface{}
		err = decoder.Decode(&raw)
		if err != nil {
			log.Errorf("Unable to decode ports response from ONOS : %s", err)
			return
		}
		wrapper, err := gabs.Consume(raw)
		if err != nil {
			log.Errorf("Unable to consume ports JSON object : %s", err)
			return
		}

		ports, err := wrapper.Path("ports").Children()
		if err != nil {
			log.Errorf("Unable to query list of ports from JSON object : %s", err)
			return
		}
		for _, port := range ports {
			if port.Path("isEnabled").Data().(bool) && port.Path("port").Data().(string) != "local" {
				inPort = port.Path("port").Data().(string)
			}
		}
		if inPort == DISCOVER {
			// Unable to discover OVS switch port
			log.Errorf("Unable to discover port on switch %s", dpid)
			return
		}
	}

	// Fetch network config to get access to the list of access device VLAN IDs
	resp, err := http.Get(fmt.Sprintf(NETCFG_URL, app.OnosConnectUrl))
	if err != nil {
		log.Warnf("Unable to read ONOS network configuration : %s", err)
		return
	}
	defer resp.Body.Close()
	if int(resp.StatusCode/100) != 2 {
		log.Errorf("Unable to query ONOS network configuration : %s", resp.Status)
		return
	}
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
	resp, err = http.Get(fmt.Sprintf(FLOWS_URL, app.OnosConnectUrl, dpid))
	if err != nil {
		log.Warnf("Unable to read ONOS flows for swtich %s: %s", dpid, err)
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
					log.Infof("[DELETE]: VLAN %s rule (%s)", vlan, flow.Path("id"))
					if !app.Verify {
						client := &http.Client{}
						req, err := http.NewRequest(http.MethodDelete,
							fmt.Sprintf(DELETE_FLOW_URL, app.OnosConnectUrl,
								dpid, flow.Path("id").Data().(string)), nil)
						if err != nil {
							log.Errorf("Unable to create DELETE request for flow rule for VLAN %s  : %s", flow.Path("id"), vlan, err)
							continue
						}
						resp, err := client.Do(req)
						if err != nil {
							log.Errorf("Unable to DELETE flow rule '%s' for VLAN %s : %s", flow.Path("id"), vlan, err)
							continue
						}
						defer resp.Body.Close()
						if int(resp.StatusCode/100) != 2 {
							log.Errorf("Error response code while DELETEing flow fule '%s' for VLAN %s : %s", flow.Path("id"), vlan, resp.Status)
							continue
						}
					}
				}
			}
		}
	}

	// Iterate over all the required VLANs and if we don't have a rule for them
	// then add them
	rule := template.New(path.Base(app.CreateFlowTemplate))
	_, err = rule.ParseFiles(app.CreateFlowTemplate)
	if err != nil {
		log.Errorf("Unable to parse rule creation template '%s' : %s", app.CreateFlowTemplate, err)
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
			DPID:   dpid,
			VlanId: vlan,
			InPort: inPort,
		}

		// Create a POST to ONOS
		buf := bytes.NewBuffer(nil)
		err := rule.Execute(buf, &data)
		if err != nil {
			log.Errorf("Unable to execute create rule template: %s", err)
		}
		if app.Verify {
			var val map[string]interface{}
			err = json.Unmarshal(buf.Bytes(), &val)
			if err != nil {
				log.Errorf("Unable to parse POST data : %s", err)
				continue
			}
			data, err := json.MarshalIndent(&val, "DATA: ", "    ")
			if err != nil {
				log.Errorf("Unable to pretty print POST data : %s", err)
				continue
			}

			log.Infof("\nDATA: %s", string(data))
		} else {
			resp, err := http.Post(fmt.Sprintf(FLOWS_URL, app.OnosConnectUrl, dpid), "application/json", buf)
			if err != nil {
				log.Errorf("Error while POSTing rule for VLAN %s to ONOS : %s", vlan, err)
				continue
			}
			defer resp.Body.Close()
			if int(resp.StatusCode/100) != 2 {
				log.Errorf("Error response code while POSTing flow rule for VLAN %s to ONOS : %s", vlan, resp.Status)
				continue
			}
		}
	}
}
