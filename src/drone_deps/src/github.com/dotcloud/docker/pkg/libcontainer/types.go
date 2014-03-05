package libcontainer

import (
	"encoding/json"
	"errors"
	"github.com/syndtr/gocapability/capability"
)

var (
	ErrUnkownNamespace  = errors.New("Unknown namespace")
	ErrUnkownCapability = errors.New("Unknown capability")
	ErrUnsupported      = errors.New("Unsupported method")
)

// namespaceList is used to convert the libcontainer types
// into the names of the files located in /proc/<pid>/ns/* for
// each namespace
var (
	namespaceList = Namespaces{}

	capabilityList = Capabilities{
		{Key: "SETPCAP", Value: capability.CAP_SETPCAP},
		{Key: "SYS_MODULE", Value: capability.CAP_SYS_MODULE},
		{Key: "SYS_RAWIO", Value: capability.CAP_SYS_RAWIO},
		{Key: "SYS_PACCT", Value: capability.CAP_SYS_PACCT},
		{Key: "SYS_ADMIN", Value: capability.CAP_SYS_ADMIN},
		{Key: "SYS_NICE", Value: capability.CAP_SYS_NICE},
		{Key: "SYS_RESOURCE", Value: capability.CAP_SYS_RESOURCE},
		{Key: "SYS_TIME", Value: capability.CAP_SYS_TIME},
		{Key: "SYS_TTY_CONFIG", Value: capability.CAP_SYS_TTY_CONFIG},
		{Key: "MKNOD", Value: capability.CAP_MKNOD},
		{Key: "AUDIT_WRITE", Value: capability.CAP_AUDIT_WRITE},
		{Key: "AUDIT_CONTROL", Value: capability.CAP_AUDIT_CONTROL},
		{Key: "MAC_OVERRIDE", Value: capability.CAP_MAC_OVERRIDE},
		{Key: "MAC_ADMIN", Value: capability.CAP_MAC_ADMIN},
		{Key: "NET_ADMIN", Value: capability.CAP_NET_ADMIN},
	}
)

type (
	Namespace struct {
		Key   string
		Value int
		File  string
	}
	Namespaces []*Namespace
)

func (ns *Namespace) String() string {
	return ns.Key
}

func (ns *Namespace) MarshalJSON() ([]byte, error) {
	return json.Marshal(ns.Key)
}

func (ns *Namespace) UnmarshalJSON(src []byte) error {
	var nsName string
	if err := json.Unmarshal(src, &nsName); err != nil {
		return err
	}
	ret := GetNamespace(nsName)
	if ret == nil {
		return ErrUnkownNamespace
	}
	*ns = *ret
	return nil
}

func GetNamespace(key string) *Namespace {
	for _, ns := range namespaceList {
		if ns.Key == key {
			return ns
		}
	}
	return nil
}

// Contains returns true if the specified Namespace is
// in the slice
func (n Namespaces) Contains(ns string) bool {
	for _, nsp := range n {
		if nsp.Key == ns {
			return true
		}
	}
	return false
}

type (
	Capability struct {
		Key   string
		Value capability.Cap
	}
	Capabilities []*Capability
)

func (c *Capability) String() string {
	return c.Key
}

func (c *Capability) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Key)
}

func (c *Capability) UnmarshalJSON(src []byte) error {
	var capName string
	if err := json.Unmarshal(src, &capName); err != nil {
		return err
	}
	ret := GetCapability(capName)
	if ret == nil {
		return ErrUnkownCapability
	}
	*c = *ret
	return nil
}

func GetCapability(key string) *Capability {
	for _, capp := range capabilityList {
		if capp.Key == key {
			return capp
		}
	}
	return nil
}

// Contains returns true if the specified Capability is
// in the slice
func (c Capabilities) Contains(capp string) bool {
	for _, cap := range c {
		if cap.Key == capp {
			return true
		}
	}
	return false
}
