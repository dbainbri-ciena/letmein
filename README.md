# Let Me In
This container is used with a VOLTHA deployment that leverages an OVS switch to
enable upstream communication with a back end DHCP and/or IGMP system.

### Description
This container does a synchronization between ONOS and the managed OVS switch.
It polls the device information from ONOS and evaluates the `accessDevice`
object under each `device` entry. From the `accessDevice` object it collects
the `vlan` values.

The container then queries the flows on the managed OVS switch, which it
created as identified by the `appId` associated with the flow. The existing
flows are evaluated to understand if there exists the required flow for each
VLAN. If there exists a rule that is no longer needed, it is deleted.

Finally, for those VLANs for which there is no existing rule a new flow rule
is `POST`ed to ONOS.

### The Rule
The rule `POST`ed to ONOS does a packet in to ONOS for traffic that arrives
on on port `1` and and matches the VLAN ID of those discovered from the ONOS
network configuration. The rule is generated from the following template
file:

```
{
    "priority": 32768,
    "appId" : "{{.AppId}}",
    "timeout": 0,
    "isPermanent": true,
    "deviceId": "{{.DPID}}",
    "treatment": {
        "instructions": [
            {
                "type": "OUTPUT",
                "port": "CONTROLLER"
            }
        ]
    },
    "selector": {
        "criteria": [
            {
                "type": "IN_PORT",
                "port": "1"
            },
            {
                "type": "VLAN_VID",
                "vlanId": "{{.VlanId}}"
            }
        ]
    }
}
```
