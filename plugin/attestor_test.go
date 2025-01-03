package plugin

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

func newTestInstance() *servers.Server {
	return &servers.Server{
		ID:         "ef079b0c-e610-4dfb-b1aa-b49f07ac48e5",
		Name:       "test",
		UserID:     "9349aff8be7545ac9d2f1d00999a23cd",
		TenantID:   "fcad67a6189847c4aecfa3c81a05783b",
		HostID:     "29d3c8c896a45aa4c34e52247875d7fefc3d94bbcc9f622b5d204362",
		Status:     "ACTIVE",
		AccessIPv4: "",
		AccessIPv6: "",
		Addresses:  map[string]interface{}{},
		Metadata:   map[string]string{},
		Created:    time.Now(),
		Updated:    time.Now(),
	}
}

const (
	correctIPv4 string = "192.168.1.1"
	wrongIPv4   string = "192.168.1.2"
	correctIPv6 string = "2001:db8::1"
	wrongIPv6   string = "2001:db8::2"
	proxyIPv4   string = "192.168.2.1"
	proxyIPv6   string = "2001:db8:1::1"
	natIPv4     string = "192.168.3.1"
	natIPv6     string = "2001:db8:2::1"
)

func TestAttest(t *testing.T) {
	var tests = []struct {
		diff        int
		limit       int
		attempt     int
		metadata    string
		status      string
		addrIPv4    string
		addrIPv6    string
		tenantID    string
		requestAddr []string
		result      bool
	}{
		// success: IPv4 only
		{0, 2, 1, "test", "ACTIVE", correctIPv4, "", "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, true},
		// success: IPv6 only
		{0, 2, 1, "test", "ACTIVE", "", correctIPv6, "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv6}, true},
		// success: both IPv4 and IPv6
		{0, 2, 1, "test", "ACTIVE", correctIPv4, correctIPv6, "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4, correctIPv6}, true},
		// success: IPv4 and wrong IPv6
		{0, 2, 1, "test", "ACTIVE", correctIPv4, wrongIPv6, "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4, wrongIPv6}, true},
		// success: IPv6 and wrong IPv4
		{0, 2, 1, "test", "ACTIVE", wrongIPv4, correctIPv6, "fcad67a6189847c4aecfa3c81a05783b", []string{wrongIPv4, correctIPv6}, true},
		// fail: too old
		{-130, 2, 1, "test", "ACTIVE", correctIPv4, "", "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, false},
		// fail: to many attempts
		{0, 2, 3, "test", "ACTIVE", correctIPv4, "", "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, false},
		// fail: wrong metadata value
		{0, 2, 1, "invalid", "ACTIVE", correctIPv4, "", "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, false},
		// fail: instance state
		{0, 2, 1, "test", "ERROR", correctIPv4, "", "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, false},
		// fail: wrong IPv4
		{0, 2, 1, "test", "ACTIVE", wrongIPv4, "", "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, false},
		// fail: wrong IPv6
		{0, 2, 1, "test", "ACTIVE", "", wrongIPv6, "fcad67a6189847c4aecfa3c81a05783b", []string{correctIPv4}, false},
		// fail: wrong project id
		{0, 2, 1, "test", "ACTIVE", correctIPv4, "", "invalid", []string{correctIPv4}, false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	role := &Role{
		Name:        "test",
		Policies:    []string{"test"},
		TTL:         time.Duration(60) * time.Second,
		MaxTTL:      time.Duration(120) * time.Second,
		Period:      time.Duration(120) * time.Second,
		MetadataKey: "vault-role",
		TenantID:    "fcad67a6189847c4aecfa3c81a05783b",
		AuthPeriod:  time.Duration(120) * time.Second,
		AuthLimit:   2,
	}

	for i, test := range tests {
		var err error

		instance := newTestInstance()
		instance.ID = fmt.Sprintf("test%d", i)
		instance.AccessIPv4 = test.addrIPv4
		instance.AccessIPv6 = test.addrIPv6
		instance.Metadata["vault-role"] = test.metadata
		instance.Status = test.status
		instance.TenantID = test.tenantID
		instance.Created = time.Now().Add(time.Duration(test.diff) * time.Second)

		for i := 0; i < test.attempt; i++ {
			err = attestor.Attest(instance, role, test.requestAddr)
		}
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestAttestMetadata(t *testing.T) {
	var tests = []struct {
		key    string
		val    string
		result bool
	}{
		{"vault-role", "test", true},
		{"invalid", "test", false},
		{"vault-role", "invalid", false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	for _, test := range tests {
		instance := newTestInstance()
		instance.Metadata[test.key] = test.val

		err := attestor.AttestMetadata(instance, "vault-role", "test")
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestAttestStatus(t *testing.T) {
	var tests = []struct {
		status string
		result bool
	}{
		{"ACTIVE", true},
		{"STOPPED", false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	for _, test := range tests {
		instance := newTestInstance()
		instance.Status = test.status

		err := attestor.AttestStatus(instance)
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestAttestAddr(t *testing.T) {
	var tests = []struct {
		accessIPv4                 string
		accessIPv6                 string
		addresses                  []string
		additionalAcceptedPrefixes []string
		request                    []string
		result                     bool
	}{
		// success: IPv4 only
		{correctIPv4, "", []string{correctIPv4}, []string{}, []string{correctIPv4}, true},
		// success: IPv6 only
		{correctIPv6, "", []string{correctIPv6}, []string{}, []string{correctIPv6}, true},
		// success: IPv4 only - not in addresses list
		{correctIPv4, "", []string{}, []string{}, []string{correctIPv4}, true},
		// success: IPv6 only - not in addresses list
		{correctIPv6, "", []string{}, []string{}, []string{correctIPv6}, true},
		// success: IPv4 only - only in addresses list
		{"", "", []string{correctIPv4}, []string{}, []string{correctIPv4}, true},
		// success: IPv6 only - only in addresses list
		{"", "", []string{correctIPv6}, []string{}, []string{correctIPv6}, true},
		// success: IPv4 - only in addresses list + other
		{"", "", []string{correctIPv4, wrongIPv4}, []string{}, []string{correctIPv4}, true},
		{"", "", []string{correctIPv4, wrongIPv6}, []string{}, []string{correctIPv4}, true},
		{"", "", []string{correctIPv4, wrongIPv4, wrongIPv6}, []string{}, []string{correctIPv4}, true},
		{"", "", []string{wrongIPv4, correctIPv4, wrongIPv6}, []string{}, []string{correctIPv4}, true},
		// fail: wrong IPv4
		{wrongIPv4, "", []string{wrongIPv4}, []string{}, []string{correctIPv4}, false},
		{wrongIPv4, "", []string{}, []string{}, []string{correctIPv4}, false},
		{"", "", []string{wrongIPv4}, []string{}, []string{correctIPv4}, false},
		{"", "", []string{wrongIPv4, "192.168.1.3"}, []string{}, []string{correctIPv4}, false},

		// simulate proxy, correct IP only in additional request addresses
		{correctIPv4, "", []string{correctIPv4}, []string{}, []string{proxyIPv4, correctIPv4}, true},
		{correctIPv4, "", []string{}, []string{}, []string{proxyIPv4, correctIPv4}, true},
		{correctIPv6, "", []string{correctIPv6}, []string{}, []string{proxyIPv6, correctIPv6}, true},
		{correctIPv6, "", []string{}, []string{}, []string{proxyIPv6, correctIPv6}, true},
		{"", "", []string{correctIPv4}, []string{}, []string{proxyIPv4, correctIPv4}, true},
		{"", "", []string{correctIPv4, wrongIPv4}, []string{}, []string{proxyIPv4, correctIPv4}, true},
		{wrongIPv4, "", []string{wrongIPv4}, []string{}, []string{proxyIPv4, correctIPv4}, false},
		{wrongIPv4, "", []string{}, []string{}, []string{proxyIPv4, correctIPv4}, false},
		{"", "", []string{wrongIPv4}, []string{}, []string{proxyIPv4, correctIPv4}, false},
		{"", "", []string{wrongIPv4, "192.168.1.3"}, []string{}, []string{proxyIPv4, correctIPv4}, false},

		// simulate NAT, correct IP only in additional accepted prefixes
		{wrongIPv4, wrongIPv6, []string{}, []string{fmt.Sprintf("%s/32", natIPv4)}, []string{natIPv4}, true},
		{wrongIPv4, wrongIPv6, []string{}, []string{"192.168.99.0/24"}, []string{natIPv4}, false},
		{wrongIPv4, wrongIPv6, []string{}, []string{fmt.Sprintf("%s/128", natIPv6)}, []string{natIPv6}, true},
		{wrongIPv4, wrongIPv6, []string{}, []string{"2001:db8:99::1"}, []string{natIPv6}, false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	for _, test := range tests {
		instance := newTestInstance()
		instance.AccessIPv4 = test.accessIPv4
		instance.AccessIPv6 = test.accessIPv6
		if len(test.addresses) > 0 {
			addresses := []interface{}{}
			for _, addr := range test.addresses {
				ipVersion := 4
				if net.ParseIP(addr).To4() == nil {
					ipVersion = 6
				}
				addresses = append(addresses, map[string]interface{}{
					"OS-EXT-IPS-MAC:mac_addr": "fa:16:3e:9e:89:be",
					"OS-EXT-IPS:type":         "fixed",
					"version":                 float64(ipVersion),
					"addr":                    addr,
				})
			}
			instance.Addresses = map[string]interface{}{
				"private": addresses,
			}
		}

		err := attestor.AttestAddr(instance, test.request, test.additionalAcceptedPrefixes)
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestAttestTenantID(t *testing.T) {
	var tests = []struct {
		tenantID string
		result   bool
	}{
		{"", true},
		{"fcad67a6189847c4aecfa3c81a05783b", true},
		{"invalid", false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	for _, test := range tests {
		instance := newTestInstance()

		err := attestor.AttestTenantID(instance, test.tenantID)
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestAttestUserID(t *testing.T) {
	var tests = []struct {
		userID string
		result bool
	}{
		{"", true},
		{"9349aff8be7545ac9d2f1d00999a23cd", true},
		{"invalid", false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	for _, test := range tests {
		instance := newTestInstance()

		err := attestor.AttestUserID(instance, test.userID)
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestVerifyAuthPeriod(t *testing.T) {
	var tests = []struct {
		diff   int
		period int
		result bool
	}{
		{0, 120, true},
		{-119, 120, true},
		{-120, 120, false},
		{-121, 120, false},
	}

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	for _, test := range tests {
		instance := newTestInstance()
		instance.Created = time.Now().Add(time.Duration(test.diff) * time.Second)
		period := time.Duration(test.period) * time.Second

		_, err := attestor.VerifyAuthPeriod(instance, period)
		if (err == nil) != test.result {
			t.Errorf("unexpected result: %v - %v", test, err)
		}
	}
}

func TestVerifyAuthLimit(t *testing.T) {
	instance := newTestInstance()
	limit := 2
	deadline := time.Now().Add(30 * time.Second)

	_, storage := newTestBackend(t)
	attestor := NewAttestor(storage)

	count, err := attestor.VerifyAuthLimit(instance, limit, deadline)
	if count != 1 || err != nil {
		t.Errorf("unexpected result: [%d] %v", count, err)
	}

	count, err = attestor.VerifyAuthLimit(instance, limit, deadline)
	if count != 2 || err != nil {
		t.Errorf("unexpected result: [%d] %v", count, err)
	}

	count, err = attestor.VerifyAuthLimit(instance, limit, deadline)
	if count != 3 || err == nil {
		t.Errorf("unexpected result: [%d]", count)
	}
}
