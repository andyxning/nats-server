// Copyright 2012-2019 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func checkOptionsEqual(t *testing.T, golden, opts *Options) {
	t.Helper()
	// Clone them so we can remove private fields that we don't
	// want to be compared.
	goldenClone := golden.Clone()
	goldenClone.inConfig, goldenClone.inCmdLine = nil, nil
	optsClone := opts.Clone()
	optsClone.inConfig, optsClone.inCmdLine = nil, nil
	if !reflect.DeepEqual(goldenClone, optsClone) {
		t.Fatalf("Options are incorrect.\nexpected: %+v\ngot: %+v", goldenClone, optsClone)
	}
}

func TestDefaultOptions(t *testing.T) {
	golden := &Options{
		Host:             DEFAULT_HOST,
		Port:             DEFAULT_PORT,
		MaxConn:          DEFAULT_MAX_CONNECTIONS,
		HTTPHost:         DEFAULT_HOST,
		PingInterval:     DEFAULT_PING_INTERVAL,
		MaxPingsOut:      DEFAULT_PING_MAX_OUT,
		TLSTimeout:       float64(TLS_TIMEOUT) / float64(time.Second),
		AuthTimeout:      float64(AUTH_TIMEOUT) / float64(time.Second),
		MaxControlLine:   MAX_CONTROL_LINE_SIZE,
		MaxPayload:       MAX_PAYLOAD_SIZE,
		MaxPending:       MAX_PENDING_SIZE,
		WriteDeadline:    DEFAULT_FLUSH_DEADLINE,
		MaxClosedClients: DEFAULT_MAX_CLOSED_CLIENTS,
		LameDuckDuration: DEFAULT_LAME_DUCK_DURATION,
		LeafNode: LeafNodeOpts{
			ReconnectInterval: DEFAULT_LEAF_NODE_RECONNECT,
		},
		ConnectErrorReports:   DEFAULT_CONNECT_ERROR_REPORTS,
		ReconnectErrorReports: DEFAULT_RECONNECT_ERROR_REPORTS,
	}

	opts := &Options{}
	setBaselineOptions(opts)

	checkOptionsEqual(t, golden, opts)
}

func TestOptions_RandomPort(t *testing.T) {
	opts := &Options{Port: RANDOM_PORT}
	setBaselineOptions(opts)

	if opts.Port != 0 {
		t.Fatalf("Process of options should have resolved random port to "+
			"zero.\nexpected: %d\ngot: %d\n", 0, opts.Port)
	}
}

func TestConfigFile(t *testing.T) {
	golden := &Options{
		ConfigFile:            "./configs/test.conf",
		Host:                  "127.0.0.1",
		Port:                  4242,
		Username:              "derek",
		Password:              "porkchop",
		AuthTimeout:           1.0,
		Debug:                 false,
		Trace:                 true,
		Logtime:               false,
		HTTPPort:              8222,
		PidFile:               "/tmp/nats-server.pid",
		ProfPort:              6543,
		Syslog:                true,
		RemoteSyslog:          "udp://foo.com:33",
		MaxControlLine:        2048,
		MaxPayload:            65536,
		MaxConn:               100,
		MaxSubs:               1000,
		MaxPending:            10000000,
		PingInterval:          60 * time.Second,
		MaxPingsOut:           3,
		WriteDeadline:         3 * time.Second,
		LameDuckDuration:      4 * time.Minute,
		ConnectErrorReports:   86400,
		ReconnectErrorReports: 5,
	}

	opts, err := ProcessConfigFile("./configs/test.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}

	checkOptionsEqual(t, golden, opts)
}

func TestTLSConfigFile(t *testing.T) {
	golden := &Options{
		ConfigFile:  "./configs/tls.conf",
		Host:        "127.0.0.1",
		Port:        4443,
		Username:    "derek",
		Password:    "foo",
		AuthTimeout: 1.0,
		TLSTimeout:  2.0,
	}
	opts, err := ProcessConfigFile("./configs/tls.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	tlsConfig := opts.TLSConfig
	if tlsConfig == nil {
		t.Fatal("Expected opts.TLSConfig to be non-nil")
	}
	opts.TLSConfig = nil
	checkOptionsEqual(t, golden, opts)

	// Now check TLSConfig a bit more closely
	// CipherSuites
	ciphers := defaultCipherSuites()
	if !reflect.DeepEqual(tlsConfig.CipherSuites, ciphers) {
		t.Fatalf("Got incorrect cipher suite list: [%+v]", tlsConfig.CipherSuites)
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("Expected MinVersion of 1.2 [%v], got [%v]", tls.VersionTLS12, tlsConfig.MinVersion)
	}
	if !tlsConfig.PreferServerCipherSuites {
		t.Fatal("Expected PreferServerCipherSuites to be true")
	}
	// Verify hostname is correct in certificate
	if len(tlsConfig.Certificates) != 1 {
		t.Fatal("Expected 1 certificate")
	}
	cert := tlsConfig.Certificates[0].Leaf
	if err := cert.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("Could not verify hostname in certificate: %v\n", err)
	}

	// Now test adding cipher suites.
	opts, err = ProcessConfigFile("./configs/tls_ciphers.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	tlsConfig = opts.TLSConfig
	if tlsConfig == nil {
		t.Fatal("Expected opts.TLSConfig to be non-nil")
	}

	// CipherSuites listed in the config - test all of them.
	ciphers = []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	}

	if !reflect.DeepEqual(tlsConfig.CipherSuites, ciphers) {
		t.Fatalf("Got incorrect cipher suite list: [%+v]", tlsConfig.CipherSuites)
	}

	// Test an unrecognized/bad cipher
	if _, err := ProcessConfigFile("./configs/tls_bad_cipher.conf"); err == nil {
		t.Fatal("Did not receive an error from a unrecognized cipher")
	}

	// Test an empty cipher entry in a config file.
	if _, err := ProcessConfigFile("./configs/tls_empty_cipher.conf"); err == nil {
		t.Fatal("Did not receive an error from empty cipher_suites")
	}

	// Test a curve preference from the config.
	curves := []tls.CurveID{
		tls.CurveP256,
	}

	// test on a file that  will load the curve preference defaults
	opts, err = ProcessConfigFile("./configs/tls_ciphers.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}

	if !reflect.DeepEqual(opts.TLSConfig.CurvePreferences, defaultCurvePreferences()) {
		t.Fatalf("Got incorrect curve preference list: [%+v]", tlsConfig.CurvePreferences)
	}

	// Test specifying a single curve preference
	opts, err = ProcessConfigFile("./configs/tls_curve_prefs.conf")
	if err != nil {
		t.Fatal("Did not receive an error from a unrecognized cipher.")
	}

	if !reflect.DeepEqual(opts.TLSConfig.CurvePreferences, curves) {
		t.Fatalf("Got incorrect cipher suite list: [%+v]", tlsConfig.CurvePreferences)
	}

	// Test an unrecognized/bad curve preference
	if _, err := ProcessConfigFile("./configs/tls_bad_curve_prefs.conf"); err == nil {
		t.Fatal("Did not receive an error from a unrecognized curve preference")
	}
	// Test an empty curve preference
	if _, err := ProcessConfigFile("./configs/tls_empty_curve_prefs.conf"); err == nil {
		t.Fatal("Did not receive an error from empty curve preferences")
	}
}

func TestMergeOverrides(t *testing.T) {
	golden := &Options{
		ConfigFile:     "./configs/test.conf",
		Host:           "127.0.0.1",
		Port:           2222,
		Username:       "derek",
		Password:       "porkchop",
		AuthTimeout:    1.0,
		Debug:          true,
		Trace:          true,
		Logtime:        false,
		HTTPPort:       DEFAULT_HTTP_PORT,
		PidFile:        "/tmp/nats-server.pid",
		ProfPort:       6789,
		Syslog:         true,
		RemoteSyslog:   "udp://foo.com:33",
		MaxControlLine: 2048,
		MaxPayload:     65536,
		MaxConn:        100,
		MaxSubs:        1000,
		MaxPending:     10000000,
		PingInterval:   60 * time.Second,
		MaxPingsOut:    3,
		Cluster: ClusterOpts{
			NoAdvertise:    true,
			ConnectRetries: 2,
		},
		WriteDeadline:         3 * time.Second,
		LameDuckDuration:      4 * time.Minute,
		ConnectErrorReports:   86400,
		ReconnectErrorReports: 5,
	}
	fopts, err := ProcessConfigFile("./configs/test.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}

	// Overrides via flags
	opts := &Options{
		Port:     2222,
		Password: "porkchop",
		Debug:    true,
		HTTPPort: DEFAULT_HTTP_PORT,
		ProfPort: 6789,
		Cluster: ClusterOpts{
			NoAdvertise:    true,
			ConnectRetries: 2,
		},
	}
	merged := MergeOptions(fopts, opts)

	checkOptionsEqual(t, golden, merged)
}

func TestRemoveSelfReference(t *testing.T) {
	url1, _ := url.Parse("nats-route://user:password@10.4.5.6:4223")
	url2, _ := url.Parse("nats-route://user:password@127.0.0.1:4223")
	url3, _ := url.Parse("nats-route://user:password@127.0.0.1:4223")

	routes := []*url.URL{url1, url2, url3}

	newroutes, err := RemoveSelfReference(4223, routes)
	if err != nil {
		t.Fatalf("Error during RemoveSelfReference: %v", err)
	}

	if len(newroutes) != 1 {
		t.Fatalf("Wrong number of routes: %d", len(newroutes))
	}

	if newroutes[0] != routes[0] {
		t.Fatalf("Self reference IP address %s in Routes", routes[0])
	}
}

func TestAllowRouteWithDifferentPort(t *testing.T) {
	url1, _ := url.Parse("nats-route://user:password@127.0.0.1:4224")
	routes := []*url.URL{url1}

	newroutes, err := RemoveSelfReference(4223, routes)
	if err != nil {
		t.Fatalf("Error during RemoveSelfReference: %v", err)
	}

	if len(newroutes) != 1 {
		t.Fatalf("Wrong number of routes: %d", len(newroutes))
	}
}

func TestRouteFlagOverride(t *testing.T) {
	routeFlag := "nats-route://ruser:top_secret@127.0.0.1:8246"
	rurl, _ := url.Parse(routeFlag)

	golden := &Options{
		ConfigFile: "./configs/srv_a.conf",
		Host:       "127.0.0.1",
		Port:       7222,
		Cluster: ClusterOpts{
			Host:        "127.0.0.1",
			Port:        7244,
			Username:    "ruser",
			Password:    "top_secret",
			AuthTimeout: 0.5,
		},
		Routes:    []*url.URL{rurl},
		RoutesStr: routeFlag,
	}

	fopts, err := ProcessConfigFile("./configs/srv_a.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}

	// Overrides via flags
	opts := &Options{
		RoutesStr: routeFlag,
	}
	merged := MergeOptions(fopts, opts)

	checkOptionsEqual(t, golden, merged)
}

func TestClusterFlagsOverride(t *testing.T) {
	routeFlag := "nats-route://ruser:top_secret@127.0.0.1:7246"
	rurl, _ := url.Parse(routeFlag)

	// In this test, we override the cluster listen string. Note that in
	// the golden options, the cluster other infos correspond to what
	// is recovered from the configuration file, this explains the
	// discrepency between ClusterListenStr and the rest.
	// The server would then process the ClusterListenStr override and
	// correctly override ClusterHost/ClustherPort/etc..
	golden := &Options{
		ConfigFile: "./configs/srv_a.conf",
		Host:       "127.0.0.1",
		Port:       7222,
		Cluster: ClusterOpts{
			Host:        "127.0.0.1",
			Port:        7244,
			ListenStr:   "nats://127.0.0.1:8224",
			Username:    "ruser",
			Password:    "top_secret",
			AuthTimeout: 0.5,
		},
		Routes: []*url.URL{rurl},
	}

	fopts, err := ProcessConfigFile("./configs/srv_a.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}

	// Overrides via flags
	opts := &Options{
		Cluster: ClusterOpts{
			ListenStr: "nats://127.0.0.1:8224",
		},
	}
	merged := MergeOptions(fopts, opts)

	checkOptionsEqual(t, golden, merged)
}

func TestRouteFlagOverrideWithMultiple(t *testing.T) {
	routeFlag := "nats-route://ruser:top_secret@127.0.0.1:8246, nats-route://ruser:top_secret@127.0.0.1:8266"
	rurls := RoutesFromStr(routeFlag)

	golden := &Options{
		ConfigFile: "./configs/srv_a.conf",
		Host:       "127.0.0.1",
		Port:       7222,
		Cluster: ClusterOpts{
			Host:        "127.0.0.1",
			Port:        7244,
			Username:    "ruser",
			Password:    "top_secret",
			AuthTimeout: 0.5,
		},
		Routes:    rurls,
		RoutesStr: routeFlag,
	}

	fopts, err := ProcessConfigFile("./configs/srv_a.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}

	// Overrides via flags
	opts := &Options{
		RoutesStr: routeFlag,
	}
	merged := MergeOptions(fopts, opts)

	checkOptionsEqual(t, golden, merged)
}

func TestDynamicPortOnListen(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/listen-1.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	if opts.Port != -1 {
		t.Fatalf("Received incorrect port %v, expected -1\n", opts.Port)
	}
	if opts.HTTPPort != -1 {
		t.Fatalf("Received incorrect monitoring port %v, expected -1\n", opts.HTTPPort)
	}
	if opts.HTTPSPort != -1 {
		t.Fatalf("Received incorrect secure monitoring port %v, expected -1\n", opts.HTTPSPort)
	}
}

func TestListenConfig(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/listen.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	setBaselineOptions(opts)

	// Normal clients
	host := "10.0.1.22"
	port := 4422
	monHost := "127.0.0.1"
	if opts.Host != host {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, host)
	}
	if opts.HTTPHost != monHost {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.HTTPHost, monHost)
	}
	if opts.Port != port {
		t.Fatalf("Received incorrect port %v, expected %v\n", opts.Port, port)
	}

	// Clustering
	clusterHost := "127.0.0.1"
	clusterPort := 4244

	if opts.Cluster.Host != clusterHost {
		t.Fatalf("Received incorrect cluster host %q, expected %q\n", opts.Cluster.Host, clusterHost)
	}
	if opts.Cluster.Port != clusterPort {
		t.Fatalf("Received incorrect cluster port %v, expected %v\n", opts.Cluster.Port, clusterPort)
	}

	// HTTP
	httpHost := "127.0.0.1"
	httpPort := 8422

	if opts.HTTPHost != httpHost {
		t.Fatalf("Received incorrect http host %q, expected %q\n", opts.HTTPHost, httpHost)
	}
	if opts.HTTPPort != httpPort {
		t.Fatalf("Received incorrect http port %v, expected %v\n", opts.HTTPPort, httpPort)
	}

	// HTTPS
	httpsPort := 9443
	if opts.HTTPSPort != httpsPort {
		t.Fatalf("Received incorrect https port %v, expected %v\n", opts.HTTPSPort, httpsPort)
	}
}

func TestListenPortOnlyConfig(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/listen_port.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	setBaselineOptions(opts)

	port := 8922

	if opts.Host != DEFAULT_HOST {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, DEFAULT_HOST)
	}
	if opts.HTTPHost != DEFAULT_HOST {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, DEFAULT_HOST)
	}
	if opts.Port != port {
		t.Fatalf("Received incorrect port %v, expected %v\n", opts.Port, port)
	}
}

func TestListenPortWithColonConfig(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/listen_port_with_colon.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	setBaselineOptions(opts)

	port := 8922

	if opts.Host != DEFAULT_HOST {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, DEFAULT_HOST)
	}
	if opts.HTTPHost != DEFAULT_HOST {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, DEFAULT_HOST)
	}
	if opts.Port != port {
		t.Fatalf("Received incorrect port %v, expected %v\n", opts.Port, port)
	}
}

func TestListenMonitoringDefault(t *testing.T) {
	opts := &Options{
		Host: "10.0.1.22",
	}
	setBaselineOptions(opts)

	host := "10.0.1.22"
	if opts.Host != host {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, host)
	}
	if opts.HTTPHost != host {
		t.Fatalf("Received incorrect host %q, expected %q\n", opts.Host, host)
	}
	if opts.Port != DEFAULT_PORT {
		t.Fatalf("Received incorrect port %v, expected %v\n", opts.Port, DEFAULT_PORT)
	}
}

func TestMultipleUsersConfig(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/multiple_users.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	setBaselineOptions(opts)
}

// Test highly depends on contents of the config file listed below. Any changes to that file
// may very well break this test.
func TestAuthorizationConfig(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/authorization.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	setBaselineOptions(opts)
	lu := len(opts.Users)
	if lu != 3 {
		t.Fatalf("Expected 3 users, got %d\n", lu)
	}
	// Build a map
	mu := make(map[string]*User)
	for _, u := range opts.Users {
		mu[u.Username] = u
	}

	// Alice
	alice, ok := mu["alice"]
	if !ok {
		t.Fatalf("Expected to see user Alice\n")
	}
	// Check for permissions details
	if alice.Permissions == nil {
		t.Fatalf("Expected Alice's permissions to be non-nil\n")
	}
	if alice.Permissions.Publish == nil {
		t.Fatalf("Expected Alice's publish permissions to be non-nil\n")
	}
	if len(alice.Permissions.Publish.Allow) != 1 {
		t.Fatalf("Expected Alice's publish permissions to have 1 element, got %d\n",
			len(alice.Permissions.Publish.Allow))
	}
	pubPerm := alice.Permissions.Publish.Allow[0]
	if pubPerm != "*" {
		t.Fatalf("Expected Alice's publish permissions to be '*', got %q\n", pubPerm)
	}
	if alice.Permissions.Subscribe == nil {
		t.Fatalf("Expected Alice's subscribe permissions to be non-nil\n")
	}
	if len(alice.Permissions.Subscribe.Allow) != 1 {
		t.Fatalf("Expected Alice's subscribe permissions to have 1 element, got %d\n",
			len(alice.Permissions.Subscribe.Allow))
	}
	subPerm := alice.Permissions.Subscribe.Allow[0]
	if subPerm != ">" {
		t.Fatalf("Expected Alice's subscribe permissions to be '>', got %q\n", subPerm)
	}

	// Bob
	bob, ok := mu["bob"]
	if !ok {
		t.Fatalf("Expected to see user Bob\n")
	}
	if bob.Permissions == nil {
		t.Fatalf("Expected Bob's permissions to be non-nil\n")
	}

	// Susan
	susan, ok := mu["susan"]
	if !ok {
		t.Fatalf("Expected to see user Susan\n")
	}
	if susan.Permissions == nil {
		t.Fatalf("Expected Susan's permissions to be non-nil\n")
	}
	// Check susan closely since she inherited the default permissions.
	if susan.Permissions == nil {
		t.Fatalf("Expected Susan's permissions to be non-nil\n")
	}
	if susan.Permissions.Publish != nil {
		t.Fatalf("Expected Susan's publish permissions to be nil\n")
	}
	if susan.Permissions.Subscribe == nil {
		t.Fatalf("Expected Susan's subscribe permissions to be non-nil\n")
	}
	if len(susan.Permissions.Subscribe.Allow) != 1 {
		t.Fatalf("Expected Susan's subscribe permissions to have 1 element, got %d\n",
			len(susan.Permissions.Subscribe.Allow))
	}
	subPerm = susan.Permissions.Subscribe.Allow[0]
	if subPerm != "PUBLIC.>" {
		t.Fatalf("Expected Susan's subscribe permissions to be 'PUBLIC.>', got %q\n", subPerm)
	}
}

// Test highly depends on contents of the config file listed below. Any changes to that file
// may very well break this test.
func TestNewStyleAuthorizationConfig(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/new_style_authorization.conf")
	if err != nil {
		t.Fatalf("Received an error reading config file: %v\n", err)
	}
	setBaselineOptions(opts)

	lu := len(opts.Users)
	if lu != 2 {
		t.Fatalf("Expected 2 users, got %d\n", lu)
	}
	// Build a map
	mu := make(map[string]*User)
	for _, u := range opts.Users {
		mu[u.Username] = u
	}
	// Alice
	alice, ok := mu["alice"]
	if !ok {
		t.Fatalf("Expected to see user Alice\n")
	}
	if alice.Permissions == nil {
		t.Fatalf("Expected Alice's permissions to be non-nil\n")
	}

	if alice.Permissions.Publish == nil {
		t.Fatalf("Expected Alice's publish permissions to be non-nil\n")
	}
	if len(alice.Permissions.Publish.Allow) != 3 {
		t.Fatalf("Expected Alice's allowed publish permissions to have 3 elements, got %d\n",
			len(alice.Permissions.Publish.Allow))
	}
	pubPerm := alice.Permissions.Publish.Allow[0]
	if pubPerm != "foo" {
		t.Fatalf("Expected Alice's first allowed publish permission to be 'foo', got %q\n", pubPerm)
	}
	pubPerm = alice.Permissions.Publish.Allow[1]
	if pubPerm != "bar" {
		t.Fatalf("Expected Alice's second allowed publish permission to be 'bar', got %q\n", pubPerm)
	}
	pubPerm = alice.Permissions.Publish.Allow[2]
	if pubPerm != "baz" {
		t.Fatalf("Expected Alice's third allowed publish permission to be 'baz', got %q\n", pubPerm)
	}
	if len(alice.Permissions.Publish.Deny) != 0 {
		t.Fatalf("Expected Alice's denied publish permissions to have 0 elements, got %d\n",
			len(alice.Permissions.Publish.Deny))
	}

	if alice.Permissions.Subscribe == nil {
		t.Fatalf("Expected Alice's subscribe permissions to be non-nil\n")
	}
	if len(alice.Permissions.Subscribe.Allow) != 0 {
		t.Fatalf("Expected Alice's allowed subscribe permissions to have 0 elements, got %d\n",
			len(alice.Permissions.Subscribe.Allow))
	}
	if len(alice.Permissions.Subscribe.Deny) != 1 {
		t.Fatalf("Expected Alice's denied subscribe permissions to have 1 element, got %d\n",
			len(alice.Permissions.Subscribe.Deny))
	}
	subPerm := alice.Permissions.Subscribe.Deny[0]
	if subPerm != "$SYSTEM.>" {
		t.Fatalf("Expected Alice's only denied subscribe permission to be '$SYSTEM.>', got %q\n", subPerm)
	}

	// Bob
	bob, ok := mu["bob"]
	if !ok {
		t.Fatalf("Expected to see user Bob\n")
	}
	if bob.Permissions == nil {
		t.Fatalf("Expected Bob's permissions to be non-nil\n")
	}

	if bob.Permissions.Publish == nil {
		t.Fatalf("Expected Bobs's publish permissions to be non-nil\n")
	}
	if len(bob.Permissions.Publish.Allow) != 1 {
		t.Fatalf("Expected Bob's allowed publish permissions to have 1 element, got %d\n",
			len(bob.Permissions.Publish.Allow))
	}
	pubPerm = bob.Permissions.Publish.Allow[0]
	if pubPerm != "$SYSTEM.>" {
		t.Fatalf("Expected Bob's first allowed publish permission to be '$SYSTEM.>', got %q\n", pubPerm)
	}
	if len(bob.Permissions.Publish.Deny) != 0 {
		t.Fatalf("Expected Bob's denied publish permissions to have 0 elements, got %d\n",
			len(bob.Permissions.Publish.Deny))
	}

	if bob.Permissions.Subscribe == nil {
		t.Fatalf("Expected Bob's subscribe permissions to be non-nil\n")
	}
	if len(bob.Permissions.Subscribe.Allow) != 0 {
		t.Fatalf("Expected Bob's allowed subscribe permissions to have 0 elements, got %d\n",
			len(bob.Permissions.Subscribe.Allow))
	}
	if len(bob.Permissions.Subscribe.Deny) != 3 {
		t.Fatalf("Expected Bobs's denied subscribe permissions to have 3 elements, got %d\n",
			len(bob.Permissions.Subscribe.Deny))
	}
	subPerm = bob.Permissions.Subscribe.Deny[0]
	if subPerm != "foo" {
		t.Fatalf("Expected Bobs's first denied subscribe permission to be 'foo', got %q\n", subPerm)
	}
	subPerm = bob.Permissions.Subscribe.Deny[1]
	if subPerm != "bar" {
		t.Fatalf("Expected Bobs's second denied subscribe permission to be 'bar', got %q\n", subPerm)
	}
	subPerm = bob.Permissions.Subscribe.Deny[2]
	if subPerm != "baz" {
		t.Fatalf("Expected Bobs's third denied subscribe permission to be 'baz', got %q\n", subPerm)
	}
}

// Test new nkey users
func TestNkeyUsersConfig(t *testing.T) {
	confFileName := createConfFile(t, []byte(`
    authorization {
      users = [
        {nkey: "UDKTV7HZVYJFJN64LLMYQBUR6MTNNYCDC3LAZH4VHURW3GZLL3FULBXV"}
        {nkey: "UA3C5TBZYK5GJQJRWPMU6NFY5JNAEVQB2V2TUZFZDHFJFUYVKTTUOFKZ"}
      ]
    }`))
	defer os.Remove(confFileName)
	opts, err := ProcessConfigFile(confFileName)
	if err != nil {
		t.Fatalf("Received an error reading config file: %v", err)
	}
	lu := len(opts.Nkeys)
	if lu != 2 {
		t.Fatalf("Expected 2 nkey users, got %d", lu)
	}
}

func TestNkeyUsersWithPermsConfig(t *testing.T) {
	confFileName := createConfFile(t, []byte(`
    authorization {
      users = [
        {nkey: "UDKTV7HZVYJFJN64LLMYQBUR6MTNNYCDC3LAZH4VHURW3GZLL3FULBXV",
         permissions = {
           publish = "$SYSTEM.>"
           subscribe = { deny = ["foo", "bar", "baz"] }
         }
        }
      ]
    }`))
	defer os.Remove(confFileName)
	opts, err := ProcessConfigFile(confFileName)
	if err != nil {
		t.Fatalf("Received an error reading config file: %v", err)
	}
	lu := len(opts.Nkeys)
	if lu != 1 {
		t.Fatalf("Expected 1 nkey user, got %d", lu)
	}
	nk := opts.Nkeys[0]
	if nk.Permissions == nil {
		t.Fatal("Expected to have permissions")
	}
	if nk.Permissions.Publish == nil {
		t.Fatal("Expected to have publish permissions")
	}
	if nk.Permissions.Publish.Allow[0] != "$SYSTEM.>" {
		t.Fatalf("Expected publish to allow \"$SYSTEM.>\", but got %v\n", nk.Permissions.Publish.Allow[0])
	}
	if nk.Permissions.Subscribe == nil {
		t.Fatal("Expected to have subscribe permissions")
	}
	if nk.Permissions.Subscribe.Allow != nil {
		t.Fatal("Expected to have no subscribe allow permissions")
	}
	deny := nk.Permissions.Subscribe.Deny
	if deny == nil || len(deny) != 3 ||
		deny[0] != "foo" || deny[1] != "bar" || deny[2] != "baz" {
		t.Fatalf("Expected to have subscribe deny permissions, got %v", deny)
	}
}

func TestBadNkeyConfig(t *testing.T) {
	confFileName := "nkeys_bad.conf"
	defer os.Remove(confFileName)
	content := `
    authorization {
      users = [ {nkey: "Ufoo"}]
    }`
	if err := ioutil.WriteFile(confFileName, []byte(content), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	if _, err := ProcessConfigFile(confFileName); err == nil {
		t.Fatalf("Expected an error from nkey entry with password")
	}
}

func TestNkeyWithPassConfig(t *testing.T) {
	confFileName := "nkeys_pass.conf"
	defer os.Remove(confFileName)
	content := `
    authorization {
      users = [
        {nkey: "UDKTV7HZVYJFJN64LLMYQBUR6MTNNYCDC3LAZH4VHURW3GZLL3FULBXV", pass: "foo"}
      ]
    }`
	if err := ioutil.WriteFile(confFileName, []byte(content), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	if _, err := ProcessConfigFile(confFileName); err == nil {
		t.Fatalf("Expected an error from bad nkey entry")
	}
}

func TestTokenWithUserPass(t *testing.T) {
	confFileName := "test.conf"
	defer os.Remove(confFileName)
	content := `
	authorization={
		user: user
		pass: password
		token: $2a$11$whatever
	}`
	if err := ioutil.WriteFile(confFileName, []byte(content), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	_, err := ProcessConfigFile(confFileName)
	if err == nil {
		t.Fatal("Expected error, got none")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("Expected error related to token, got %v", err)
	}
}

func TestTokenWithUsers(t *testing.T) {
	confFileName := "test.conf"
	defer os.Remove(confFileName)
	content := `
	authorization={
		token: $2a$11$whatever
		users: [
			{user: test, password: test}
		]
	}`
	if err := ioutil.WriteFile(confFileName, []byte(content), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	_, err := ProcessConfigFile(confFileName)
	if err == nil {
		t.Fatal("Expected error, got none")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("Expected error related to token, got %v", err)
	}
}

func TestParseWriteDeadline(t *testing.T) {
	confFile := "test.conf"
	defer os.Remove(confFile)
	if err := ioutil.WriteFile(confFile, []byte("write_deadline: \"1x\"\n"), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	_, err := ProcessConfigFile(confFile)
	if err == nil {
		t.Fatal("Expected error, got none")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("Expected error related to parsing, got %v", err)
	}
	os.Remove(confFile)
	if err := ioutil.WriteFile(confFile, []byte("write_deadline: \"1s\"\n"), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	opts, err := ProcessConfigFile(confFile)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if opts.WriteDeadline != time.Second {
		t.Fatalf("Expected write_deadline to be 1s, got %v", opts.WriteDeadline)
	}
	os.Remove(confFile)
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	defer func() {
		w.Close()
		os.Stdout = oldStdout
	}()
	os.Stdout = w
	if err := ioutil.WriteFile(confFile, []byte("write_deadline: 2\n"), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	opts, err = ProcessConfigFile(confFile)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if opts.WriteDeadline != 2*time.Second {
		t.Fatalf("Expected write_deadline to be 2s, got %v", opts.WriteDeadline)
	}
}

func TestOptionsClone(t *testing.T) {
	opts := &Options{
		ConfigFile:     "./configs/test.conf",
		Host:           "127.0.0.1",
		Port:           2222,
		Username:       "derek",
		Password:       "porkchop",
		AuthTimeout:    1.0,
		Debug:          true,
		Trace:          true,
		Logtime:        false,
		HTTPPort:       DEFAULT_HTTP_PORT,
		PidFile:        "/tmp/nats-server.pid",
		ProfPort:       6789,
		Syslog:         true,
		RemoteSyslog:   "udp://foo.com:33",
		MaxControlLine: 2048,
		MaxPayload:     65536,
		MaxConn:        100,
		PingInterval:   60 * time.Second,
		MaxPingsOut:    3,
		Cluster: ClusterOpts{
			NoAdvertise:    true,
			ConnectRetries: 2,
		},
		Gateway: GatewayOpts{
			Name: "A",
			Gateways: []*RemoteGatewayOpts{
				{Name: "B", URLs: []*url.URL{{Scheme: "nats", Host: "host:5222"}}},
				{Name: "C"},
			},
		},
		WriteDeadline: 3 * time.Second,
		Routes:        []*url.URL{{}},
		Users:         []*User{{Username: "foo", Password: "bar"}},
	}

	clone := opts.Clone()

	if !reflect.DeepEqual(opts, clone) {
		t.Fatalf("Cloned Options are incorrect.\nexpected: %+v\ngot: %+v",
			clone, opts)
	}

	clone.Users[0].Password = "baz"
	if reflect.DeepEqual(opts, clone) {
		t.Fatal("Expected Options to be different")
	}

	opts.Gateway.Gateways[0].URLs[0] = nil
	if reflect.DeepEqual(opts.Gateway.Gateways[0], clone.Gateway.Gateways[0]) {
		t.Fatal("Expected Options to be different")
	}
	if clone.Gateway.Gateways[0].URLs[0].Host != "host:5222" {
		t.Fatalf("Unexpected URL: %v", clone.Gateway.Gateways[0].URLs[0])
	}
}

func TestOptionsCloneNilLists(t *testing.T) {
	opts := &Options{}

	clone := opts.Clone()

	if clone.Routes != nil {
		t.Fatalf("Expected Routes to be nil, got: %v", clone.Routes)
	}
	if clone.Users != nil {
		t.Fatalf("Expected Users to be nil, got: %v", clone.Users)
	}
}

func TestOptionsCloneNil(t *testing.T) {
	opts := (*Options)(nil)
	clone := opts.Clone()
	if clone != nil {
		t.Fatalf("Expected nil, got: %+v", clone)
	}
}

func TestEmptyConfig(t *testing.T) {
	opts, err := ProcessConfigFile("")

	if err != nil {
		t.Fatalf("Expected no error from empty config, got: %+v", err)
	}

	if opts.ConfigFile != "" {
		t.Fatalf("Expected empty config, got: %+v", opts)
	}
}

func TestMalformedListenAddress(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/malformed_listen_address.conf")
	if err == nil {
		t.Fatalf("Expected an error reading config file: got %+v\n", opts)
	}
}

func TestMalformedClusterAddress(t *testing.T) {
	opts, err := ProcessConfigFile("./configs/malformed_cluster_address.conf")
	if err == nil {
		t.Fatalf("Expected an error reading config file: got %+v\n", opts)
	}
}

func TestOptionsProcessConfigFile(t *testing.T) {
	// Create options with default values of Debug and Trace
	// that are the opposite of what is in the config file.
	// Set another option that is not present in the config file.
	logFileName := "test.log"
	opts := &Options{
		Debug:   true,
		Trace:   false,
		LogFile: logFileName,
	}
	configFileName := "./configs/test.conf"
	if err := opts.ProcessConfigFile(configFileName); err != nil {
		t.Fatalf("Error processing config file: %v", err)
	}
	// Verify that values are as expected
	if opts.ConfigFile != configFileName {
		t.Fatalf("Expected ConfigFile to be set to %q, got %v", configFileName, opts.ConfigFile)
	}
	if opts.Debug {
		t.Fatal("Debug option should have been set to false from config file")
	}
	if !opts.Trace {
		t.Fatal("Trace option should have been set to true from config file")
	}
	if opts.LogFile != logFileName {
		t.Fatalf("Expected LogFile to be %q, got %q", logFileName, opts.LogFile)
	}
}

func TestConfigureOptions(t *testing.T) {
	// Options.Configure() will snapshot the flags. This is used by the reload code.
	// We need to set it back to nil otherwise it will impact reload tests.
	defer func() { FlagSnapshot = nil }()

	ch := make(chan bool, 1)
	checkPrintInvoked := func() {
		ch <- true
	}
	usage := func() { panic("should not get there") }
	var fs *flag.FlagSet
	type testPrint struct {
		args                   []string
		version, help, tlsHelp func()
	}
	testFuncs := []testPrint{
		{[]string{"-v"}, checkPrintInvoked, usage, PrintTLSHelpAndDie},
		{[]string{"version"}, checkPrintInvoked, usage, PrintTLSHelpAndDie},
		{[]string{"-h"}, PrintServerAndExit, checkPrintInvoked, PrintTLSHelpAndDie},
		{[]string{"help"}, PrintServerAndExit, checkPrintInvoked, PrintTLSHelpAndDie},
		{[]string{"-help_tls"}, PrintServerAndExit, usage, checkPrintInvoked},
	}
	for _, tf := range testFuncs {
		fs = flag.NewFlagSet("test", flag.ContinueOnError)
		opts, err := ConfigureOptions(fs, tf.args, tf.version, tf.help, tf.tlsHelp)
		if err != nil {
			t.Fatalf("Error on configure: %v", err)
		}
		if opts != nil {
			t.Fatalf("Expected options to be nil, got %v", opts)
		}
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("Should have invoked print function for args=%v", tf.args)
		}
	}

	// Helper function that expect parsing with given args to not produce an error.
	mustNotFail := func(args []string) *Options {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		opts, err := ConfigureOptions(fs, args, PrintServerAndExit, fs.Usage, PrintTLSHelpAndDie)
		if err != nil {
			stackFatalf(t, "Error on configure: %v", err)
		}
		return opts
	}

	// Helper function that expect configuration to fail.
	expectToFail := func(args []string, errContent ...string) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		// Silence the flagSet so that on failure nothing is printed.
		// (flagSet would print error message about unknown flags, etc..)
		silenceOuput := &bytes.Buffer{}
		fs.SetOutput(silenceOuput)
		opts, err := ConfigureOptions(fs, args, PrintServerAndExit, fs.Usage, PrintTLSHelpAndDie)
		if opts != nil || err == nil {
			stackFatalf(t, "Expected no option and an error, got opts=%v and err=%v", opts, err)
		}
		for _, testErr := range errContent {
			if strings.Contains(err.Error(), testErr) {
				// We got the error we wanted.
				return
			}
		}
		stackFatalf(t, "Expected errors containing any of those %v, got %v", errContent, err)
	}

	// Basic test with port number
	opts := mustNotFail([]string{"-p", "1234"})
	if opts.Port != 1234 {
		t.Fatalf("Expected port to be 1234, got %v", opts.Port)
	}

	// Should fail because of unknown parameter
	expectToFail([]string{"foo"}, "command")

	// Should fail because unknown flag
	expectToFail([]string{"-xxx", "foo"}, "flag")

	// Should fail because of config file missing
	expectToFail([]string{"-c", "xxx.cfg"}, "file")

	// Should fail because of too many args for signal command
	expectToFail([]string{"-sl", "quit=pid=foo"}, "signal")

	// Should fail because of invalid pid
	// On windows, if not running with admin privileges, you would get access denied.
	expectToFail([]string{"-sl", "quit=pid"}, "pid", "denied")

	// The config file set Trace to true.
	opts = mustNotFail([]string{"-c", "./configs/test.conf"})
	if !opts.Trace {
		t.Fatal("Trace should have been set to true")
	}

	// The config file set Trace to true, but was overridden by param -V=false
	opts = mustNotFail([]string{"-c", "./configs/test.conf", "-V=false"})
	if opts.Trace {
		t.Fatal("Trace should have been set to false")
	}

	// The config file set Trace to true, but was overridden by param -DV=false
	opts = mustNotFail([]string{"-c", "./configs/test.conf", "-DV=false"})
	if opts.Debug || opts.Trace {
		t.Fatal("Debug and Trace should have been set to false")
	}

	// The config file set Trace to true, but was overridden by param -DV
	opts = mustNotFail([]string{"-c", "./configs/test.conf", "-DV"})
	if !opts.Debug || !opts.Trace {
		t.Fatal("Debug and Trace should have been set to true")
	}

	// This should fail since -cluster is missing
	expectedURL, _ := url.Parse("nats://127.0.0.1:6223")
	expectToFail([]string{"-routes", expectedURL.String()}, "solicited routes")

	// Ensure that we can set cluster and routes from command line
	opts = mustNotFail([]string{"-cluster", "nats://127.0.0.1:6222", "-routes", expectedURL.String()})
	if opts.Cluster.ListenStr != "nats://127.0.0.1:6222" {
		t.Fatalf("Unexpected Cluster.ListenStr=%q", opts.Cluster.ListenStr)
	}
	if opts.RoutesStr != "nats://127.0.0.1:6223" || len(opts.Routes) != 1 || opts.Routes[0].String() != expectedURL.String() {
		t.Fatalf("Unexpected RoutesStr: %q and Routes: %v", opts.RoutesStr, opts.Routes)
	}

	// Use a config with cluster configuration and explicit route defined.
	// Override with empty routes string.
	opts = mustNotFail([]string{"-c", "./configs/srv_a.conf", "-routes", ""})
	if opts.RoutesStr != "" || len(opts.Routes) != 0 {
		t.Fatalf("Unexpected RoutesStr: %q and Routes: %v", opts.RoutesStr, opts.Routes)
	}

	// Use a config with cluster configuration and override cluster listen string
	expectedURL, _ = url.Parse("nats-route://ruser:top_secret@127.0.0.1:7246")
	opts = mustNotFail([]string{"-c", "./configs/srv_a.conf", "-cluster", "nats://ivan:pwd@127.0.0.1:6222"})
	if opts.Cluster.Username != "ivan" || opts.Cluster.Password != "pwd" || opts.Cluster.Port != 6222 ||
		len(opts.Routes) != 1 || opts.Routes[0].String() != expectedURL.String() {
		t.Fatalf("Unexpected Cluster and/or Routes: %#v - %v", opts.Cluster, opts.Routes)
	}

	// Disable clustering from command line
	opts = mustNotFail([]string{"-c", "./configs/srv_a.conf", "-cluster", ""})
	if opts.Cluster.Port != 0 {
		t.Fatalf("Unexpected Cluster: %v", opts.Cluster)
	}

	// Various erros due to malformed cluster listen string.
	// (adding -routes to have more than 1 set flag to check
	// that Visit() stops when an error is found).
	expectToFail([]string{"-cluster", ":", "-routes", ""}, "protocol")
	expectToFail([]string{"-cluster", "nats://127.0.0.1", "-routes", ""}, "port")
	expectToFail([]string{"-cluster", "nats://127.0.0.1:xxx", "-routes", ""}, "integer")
	expectToFail([]string{"-cluster", "nats://ivan:127.0.0.1:6222", "-routes", ""}, "colons")
	expectToFail([]string{"-cluster", "nats://ivan@127.0.0.1:6222", "-routes", ""}, "password")

	// Override config file's TLS configuration from command line, and completely disable TLS
	opts = mustNotFail([]string{"-c", "./configs/tls.conf", "-tls=false"})
	if opts.TLSConfig != nil || opts.TLS {
		t.Fatal("Expected TLS to be disabled")
	}
	// Override config file's TLS configuration from command line, and force TLS verification.
	// However, since TLS config has to be regenerated, user need to provide -tlscert and -tlskey too.
	// So this should fail.
	expectToFail([]string{"-c", "./configs/tls.conf", "-tlsverify"}, "valid")

	// Now same than above, but with all valid params.
	opts = mustNotFail([]string{"-c", "./configs/tls.conf", "-tlsverify", "-tlscert", "./configs/certs/server.pem", "-tlskey", "./configs/certs/key.pem"})
	if opts.TLSConfig == nil || !opts.TLSVerify {
		t.Fatal("Expected TLS to be configured and force verification")
	}

	// Configure TLS, but some TLS params missing
	expectToFail([]string{"-tls"}, "valid")
	expectToFail([]string{"-tls", "-tlscert", "./configs/certs/server.pem"}, "valid")
	// One of the file does not exist
	expectToFail([]string{"-tls", "-tlscert", "./configs/certs/server.pem", "-tlskey", "./configs/certs/notfound.pem"}, "file")

	// Configure TLS and check that this results in a TLSConfig option.
	opts = mustNotFail([]string{"-tls", "-tlscert", "./configs/certs/server.pem", "-tlskey", "./configs/certs/key.pem"})
	if opts.TLSConfig == nil || !opts.TLS {
		t.Fatal("Expected TLSConfig to be set")
	}
}

func TestClusterPermissionsConfig(t *testing.T) {
	template := `
		cluster {
			port: 1234
			%s
			authorization {
				user: ivan
				password: pwd
				permissions {
					import {
						allow: "foo"
					}
					export {
						allow: "bar"
					}
				}
			}
		}
	`
	conf := createConfFile(t, []byte(fmt.Sprintf(template, "")))
	defer os.Remove(conf)
	opts, err := ProcessConfigFile(conf)
	if err != nil {
		if cerr, ok := err.(*processConfigErr); ok && len(cerr.Errors()) > 0 {
			t.Fatalf("Error processing config file: %v", err)
		}
	}
	if opts.Cluster.Permissions == nil {
		t.Fatal("Expected cluster permissions to be set")
	}
	if opts.Cluster.Permissions.Import == nil {
		t.Fatal("Expected cluster import permissions to be set")
	}
	if len(opts.Cluster.Permissions.Import.Allow) != 1 || opts.Cluster.Permissions.Import.Allow[0] != "foo" {
		t.Fatalf("Expected cluster import permissions to have %q, got %v", "foo", opts.Cluster.Permissions.Import.Allow)
	}
	if opts.Cluster.Permissions.Export == nil {
		t.Fatal("Expected cluster export permissions to be set")
	}
	if len(opts.Cluster.Permissions.Export.Allow) != 1 || opts.Cluster.Permissions.Export.Allow[0] != "bar" {
		t.Fatalf("Expected cluster export permissions to have %q, got %v", "bar", opts.Cluster.Permissions.Export.Allow)
	}

	// Now add permissions in top level cluster and check
	// that this is the one that is being used.
	conf = createConfFile(t, []byte(fmt.Sprintf(template, `
		permissions {
			import {
				allow: "baz"
			}
			export {
				allow: "bat"
			}
		}
	`)))
	defer os.Remove(conf)
	opts, err = ProcessConfigFile(conf)
	if err != nil {
		t.Fatalf("Error processing config file: %v", err)
	}
	if opts.Cluster.Permissions == nil {
		t.Fatal("Expected cluster permissions to be set")
	}
	if opts.Cluster.Permissions.Import == nil {
		t.Fatal("Expected cluster import permissions to be set")
	}
	if len(opts.Cluster.Permissions.Import.Allow) != 1 || opts.Cluster.Permissions.Import.Allow[0] != "baz" {
		t.Fatalf("Expected cluster import permissions to have %q, got %v", "baz", opts.Cluster.Permissions.Import.Allow)
	}
	if opts.Cluster.Permissions.Export == nil {
		t.Fatal("Expected cluster export permissions to be set")
	}
	if len(opts.Cluster.Permissions.Export.Allow) != 1 || opts.Cluster.Permissions.Export.Allow[0] != "bat" {
		t.Fatalf("Expected cluster export permissions to have %q, got %v", "bat", opts.Cluster.Permissions.Export.Allow)
	}

	// Tests with invalid permissions
	invalidPerms := []string{
		`permissions: foo`,
		`permissions {
			unknown_field: "foo"
		}`,
		`permissions {
			import: [1, 2, 3]
		}`,
		`permissions {
			import {
				unknown_field: "foo"
			}
		}`,
		`permissions {
			import {
				allow {
					x: y
				}
			}
		}`,
		`permissions {
			import {
				deny {
					x: y
				}
			}
		}`,
		`permissions {
			export: [1, 2, 3]
		}`,
		`permissions {
			export {
				unknown_field: "foo"
			}
		}`,
		`permissions {
			export {
				allow {
					x: y
				}
			}
		}`,
		`permissions {
			export {
				deny {
					x: y
				}
			}
		}`,
	}
	for _, perms := range invalidPerms {
		conf = createConfFile(t, []byte(fmt.Sprintf(`
			cluster {
				port: 1234
				%s
			}
		`, perms)))
		_, err := ProcessConfigFile(conf)
		os.Remove(conf)
		if err == nil {
			t.Fatalf("Expected failure for permissions %s", perms)
		}
	}

	for _, perms := range invalidPerms {
		conf = createConfFile(t, []byte(fmt.Sprintf(`
			cluster {
				port: 1234
				authorization {
					user: ivan
					password: pwd
					%s
				}
			}
		`, perms)))
		_, err := ProcessConfigFile(conf)
		os.Remove(conf)
		if err == nil {
			t.Fatalf("Expected failure for permissions %s", perms)
		}
	}
}

func TestAccountUsersLoadedProperly(t *testing.T) {
	conf := createConfFile(t, []byte(`
	listen: "127.0.0.1:-1"
	authorization {
		users [
			{user: ivan, password: bar}
			{nkey : UC6NLCN7AS34YOJVCYD4PJ3QB7QGLYG5B5IMBT25VW5K4TNUJODM7BOX}
		]
	}
	accounts {
		synadia {
			users [
				{user: derek, password: foo}
				{nkey : UBAAQWTW6CG2G6ANGNKB5U2B7HRWHSGMZEZX3AQSAJOQDAUGJD46LD2E}
			]
		}
	}
	`))
	check := func(t *testing.T) {
		t.Helper()
		s, _ := RunServerWithConfig(conf)
		defer s.Shutdown()
		opts := s.getOpts()
		if n := len(opts.Users); n != 2 {
			t.Fatalf("Should have 2 users, got %v", n)
		}
		if n := len(opts.Nkeys); n != 2 {
			t.Fatalf("Should have 2 nkeys, got %v", n)
		}
	}
	// Repeat test since issue was with ordering of processing
	// of authorization vs accounts that depends on range of a map (after actual parsing)
	for i := 0; i < 20; i++ {
		check(t)
	}
}

func TestParsingGateways(t *testing.T) {
	content := `
	gateway {
		name: "A"
		listen: "127.0.0.1:4444"
		host: "127.0.0.1"
		port: 4444
		authorization {
			user: "ivan"
			password: "pwd"
			timeout: 2.0
		}
		tls {
			cert_file: "./configs/certs/server.pem"
			key_file: "./configs/certs/key.pem"
			timeout: 3.0
		}
		advertise: "me:1"
		connect_retries: 10
		gateways: [
			{
				name: "B"
				urls: ["nats://user1:pwd1@host2:5222", "nats://user1:pwd1@host3:6222"]
			}
			{
				name: "C"
				url: "nats://host4:7222"
			}
		]
	}
	`
	file := "server_config_gateways.conf"
	defer os.Remove(file)
	if err := ioutil.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	opts, err := ProcessConfigFile(file)
	if err != nil {
		t.Fatalf("Error processing file: %v", err)
	}

	expected := &GatewayOpts{
		Name:           "A",
		Host:           "127.0.0.1",
		Port:           4444,
		Username:       "ivan",
		Password:       "pwd",
		AuthTimeout:    2.0,
		Advertise:      "me:1",
		ConnectRetries: 10,
		TLSTimeout:     3.0,
	}
	u1, _ := url.Parse("nats://user1:pwd1@host2:5222")
	u2, _ := url.Parse("nats://user1:pwd1@host3:6222")
	urls := []*url.URL{u1, u2}
	gw := &RemoteGatewayOpts{
		Name: "B",
		URLs: urls,
	}
	expected.Gateways = append(expected.Gateways, gw)

	u1, _ = url.Parse("nats://host4:7222")
	urls = []*url.URL{u1}
	gw = &RemoteGatewayOpts{
		Name: "C",
		URLs: urls,
	}
	expected.Gateways = append(expected.Gateways, gw)

	// Just make sure that TLSConfig is set.. we have aother test
	// to check proper generating TLSConfig from config file...
	if opts.Gateway.TLSConfig == nil {
		t.Fatalf("Expected TLSConfig, got none")
	}
	opts.Gateway.TLSConfig = nil
	if !reflect.DeepEqual(&opts.Gateway, expected) {
		t.Fatalf("Expected %v, got %v", expected, opts.Gateway)
	}
}

func TestParsingGatewaysErrors(t *testing.T) {
	for _, test := range []struct {
		name        string
		content     string
		expectedErr string
	}{
		{
			"bad_type",
			`gateway: "bad_type"`,
			"Expected gateway to be a map",
		},
		{
			"bad_listen",
			`gateway {
				name: "A"
				port: -1
				listen: "bad::address"
			}`,
			"parse address",
		},
		{
			"bad_auth",
			`gateway {
				name: "A"
				port: -1
				authorization {
					users {
					}
				}
			}`,
			"be an array",
		},
		{
			"unknown_field",
			`gateway {
				name: "A"
				port: -1
				reject_unknown: true
				unknown_field: 1
			}`,
			"unknown field",
		},
		{
			"users_not_supported",
			`gateway {
				name: "A"
				port: -1
				authorization {
					users [
						{user: alice, password: foo}
						{user: bob,   password: bar}
					]
				}
			}`,
			"does not allow multiple users",
		},
		{
			"tls_error",
			`gateway {
				name: "A"
				port: -1
				tls {
					cert_file: 123
				}
			}`,
			"to be filename",
		},
		{
			"tls_gen_error_cert_file_not_found",
			`gateway {
				name: "A"
				port: -1
				tls {
					cert_file: "./configs/certs/missing.pem"
					key_file: "./configs/certs/server-key.pem"
				}
			}`,
			"certificate/key pair",
		},
		{
			"tls_gen_error_key_file_not_found",
			`gateway {
				name: "A"
				port: -1
				tls {
					cert_file: "./configs/certs/server.pem"
					key_file: "./configs/certs/missing.pem"
				}
			}`,
			"certificate/key pair",
		},
		{
			"tls_gen_error_key_file_missing",
			`gateway {
				name: "A"
				port: -1
				tls {
					cert_file: "./configs/certs/server.pem"
				}
			}`,
			`missing 'key_file' in TLS configuration`,
		},
		{
			"tls_gen_error_cert_file_missing",
			`gateway {
				name: "A"
				port: -1
				tls {
					key_file: "./configs/certs/server-key.pem"
				}
			}`,
			`missing 'cert_file' in TLS configuration`,
		},
		{
			"tls_gen_error_key_file_not_found",
			`gateway {
				name: "A"
				port: -1
				tls {
					cert_file: "./configs/certs/server.pem"
					key_file: "./configs/certs/missing.pem"
				}
			}`,
			"certificate/key pair",
		},
		{
			"gateways_needs_to_be_an_array",
			`gateway {
				name: "A"
				gateways {
					name: "B"
				}
			}`,
			"Expected gateways field to be an array",
		},
		{
			"gateways_entry_needs_to_be_a_map",
			`gateway {
				name: "A"
				gateways [
					"g1", "g2"
				]
			}`,
			"Expected gateway entry to be a map",
		},
		{
			"bad_url",
			`gateway {
				name: "A"
				gateways [
					{
						name: "B"
						url: "nats://wrong url"
					}
				]
			}`,
			"error parsing gateway url",
		},
		{
			"bad_urls",
			`gateway {
				name: "A"
				gateways [
					{
						name: "B"
						urls: ["nats://wrong url", "nats://host:5222"]
					}
				]
			}`,
			"error parsing gateway url",
		},
		{
			"gateway_tls_error",
			`gateway {
				name: "A"
				port: -1
				gateways [
					{
						name: "B"
						tls {
							cert_file: 123
						}
					}
				]
			}`,
			"to be filename",
		},
		{
			"gateway_unknown_field",
			`gateway {
				name: "A"
				port: -1
				gateways [
					{
						name: "B"
						unknown_field: 1
					}
				]
			}`,
			"unknown field",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := fmt.Sprintf("server_config_gateways_%s.conf", test.name)
			defer os.Remove(file)
			if err := ioutil.WriteFile(file, []byte(test.content), 0600); err != nil {
				t.Fatalf("Error writing config file: %v", err)
			}
			_, err := ProcessConfigFile(file)
			if err == nil {
				t.Fatalf("Expected to fail, did not. Content:\n%s\n", test.content)
			} else if !strings.Contains(err.Error(), test.expectedErr) {
				t.Fatalf("Expected error containing %q, got %q, for content:\n%s\n", test.expectedErr, err, test.content)
			}
		})
	}
}

func TestParsingLeafNodesListener(t *testing.T) {
	content := `
	leafnodes {
		listen: "127.0.0.1:3333"
		host: "127.0.0.1"
		port: 3333
		advertise: "me:22"
		authorization {
			user: "derek"
			password: "s3cr3t!"
			timeout: 2.2
		}
		tls {
			cert_file: "./configs/certs/server.pem"
			key_file: "./configs/certs/key.pem"
			timeout: 3.3
		}
	}
	`
	conf := createConfFile(t, []byte(content))
	defer os.Remove(conf)
	opts, err := ProcessConfigFile(conf)
	if err != nil {
		t.Fatalf("Error processing file: %v", err)
	}

	expected := &LeafNodeOpts{
		Host:        "127.0.0.1",
		Port:        3333,
		Username:    "derek",
		Password:    "s3cr3t!",
		AuthTimeout: 2.2,
		Advertise:   "me:22",
		TLSTimeout:  3.3,
	}
	if opts.LeafNode.TLSConfig == nil {
		t.Fatalf("Expected TLSConfig, got none")
	}
	opts.LeafNode.TLSConfig = nil
	if !reflect.DeepEqual(&opts.LeafNode, expected) {
		t.Fatalf("Expected %v, got %v", expected, opts.LeafNode)
	}
}

func TestParsingLeafNodeRemotes(t *testing.T) {
	content := `
		leafnodes {
			remotes = [
				{
					url: nats-leaf://127.0.0.1:2222
					account: foobar // Local Account to bind to..
					credentials: "./my.creds"
				}
			]
		}
		`
	conf := createConfFile(t, []byte(content))
	defer os.Remove(conf)
	opts, err := ProcessConfigFile(conf)
	if err != nil {
		t.Fatalf("Error processing file: %v", err)
	}
	if len(opts.LeafNode.Remotes) != 1 {
		t.Fatalf("Expected 1 remote, got %d", len(opts.LeafNode.Remotes))
	}
	expected := &RemoteLeafOpts{
		LocalAccount: "foobar",
		Credentials:  "./my.creds",
	}
	expected.URL, _ = url.Parse("nats-leaf://127.0.0.1:2222")
	if !reflect.DeepEqual(opts.LeafNode.Remotes[0], expected) {
		t.Fatalf("Expected %v, got %v", expected, opts.LeafNode.Remotes[0])
	}
}

func TestLargeMaxControlLine(t *testing.T) {
	confFileName := "big_mcl.conf"
	defer os.Remove(confFileName)
	content := `
    max_control_line = 3000000000
    `
	if err := ioutil.WriteFile(confFileName, []byte(content), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	if _, err := ProcessConfigFile(confFileName); err == nil {
		t.Fatalf("Expected an error from too large of a max_control_line entry")
	}
}

func TestLargeMaxPayload(t *testing.T) {
	confFileName := "big_mp.conf"
	defer os.Remove(confFileName)
	content := `
    max_payload = 3000000000
    `
	if err := ioutil.WriteFile(confFileName, []byte(content), 0666); err != nil {
		t.Fatalf("Error writing config file: %v", err)
	}
	if _, err := ProcessConfigFile(confFileName); err == nil {
		t.Fatalf("Expected an error from too large of a max_payload entry")
	}
}

func TestHandleUnknownTopLevelConfigurationField(t *testing.T) {
	conf := createConfFile(t, []byte(`
		port: 1234
		streaming {
			id: "me"
		}
	`))
	defer os.Remove(conf)

	// Verify that we get an error because of unknown "streaming" field.
	opts := &Options{}
	if err := opts.ProcessConfigFile(conf); err == nil || !strings.Contains(err.Error(), "streaming") {
		t.Fatal("Expected error, got none")
	}

	// Verify that if that is set, we get no error
	NoErrOnUnknownFields(true)
	defer NoErrOnUnknownFields(false)

	if err := opts.ProcessConfigFile(conf); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if opts.Port != 1234 {
		t.Fatalf("Port was not parsed correctly: %v", opts.Port)
	}

	// Verify that ignore works only on top level fields.
	changeCurrentConfigContentWithNewContent(t, conf, []byte(`
		port: 1234
		cluster {
			non_top_level_unknown_field: 123
		}
		streaming {
			id: "me"
		}
	`))
	if err := opts.ProcessConfigFile(conf); err == nil || !strings.Contains(err.Error(), "non_top_level") {
		t.Fatal("Expected error, got none")
	}
}
