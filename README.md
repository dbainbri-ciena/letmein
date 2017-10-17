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

### configuration
This container is configured via environment variables

| KEY | VALUE | DESCRIPTION |
| --- | --- | --- |
| `ONOS_CONNECT_URL` | `http://karaf:karaf@onos:8181` | URL with which to connect to ONOS |
| `OVS_DPID` | `:discover` | DPID of switch to manager |
| `OVS_PORT` | `:discover` | Port on OVS switch to provision |
| `CREATE_FLOW_TEMPLATE` | `/var/templates/create.tmpl` | Template file used to create flow rule in ONOS |
 | `INTERVAL` | `30s` | Frequency to check for correct flows |
| `VERIFY` | `false` | When true, just log changes that would be made, but don't make changes |
| `LOG_LEVEL` | `info` | detail level for logging |
| `LOG_FORMAT` | `text` | log output format, text or json |

The value `:discover` for the options `OVS_DPID` and `OVS_PORT` is used to
indicate to the container that heuristics should be used to identify the
OVS switch and its port in ONOS. The heuristics used are:
- `OVS_DPID` - based on the device attributes, as queried from ONOS, the first
  switch found where the "hw" value is "Open vSwitch", the "driver" value is
  "ovs", and the device is "available" is selected.
- `OVS_PORT` - based on the port attributes, as queried from ONOS, the first
  port on the selected switch where the port is not "local" and it is "enabled"
  is selected.
