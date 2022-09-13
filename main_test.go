package main

import (
	"os"
	"testing"
	"time"

	"github.com/jetstack/cert-manager/test/acme/dns"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
	fqdn string
)

func TestRunsSuite(t *testing.T) {
	fqdn = "_acme-challenge." + zone

	fixture := dns.NewFixture(&dnsServicesProviderSolver{},
		dns.SetResolvedZone(zone),
		dns.SetResolvedFQDN(fqdn),
		dns.SetAllowAmbientCredentials(false),
		dns.SetManifestPath("testdata/dns-services"),
		dns.SetDNSServer("195.242.131.6:53"),
		dns.SetDNSChallengeKey("dns-services-webhook-challenge-test"),
		dns.SetPollInterval(10*time.Second),
		dns.SetStrict(true),
	)

	fixture.RunBasic(t)
	fixture.RunExtended(t)

}
