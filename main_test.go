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

	fixture := dns.NewFixture(&customDNSProviderSolver{},
		dns.SetResolvedZone(zone),
		dns.SetResolvedFQDN(fqdn),
		dns.SetAllowAmbientCredentials(false),
		dns.SetManifestPath("testdata/dns-services"),
		dns.SetDNSServer("195.242.131.6:53"),
		dns.SetDNSChallengeKey("dns-services-webhook-challenge-test"),
		dns.SetPollInterval(2*time.Second),
		//dns.SetBinariesPath("_test/kubebuilder/bin"),
	)
	//solver := example.New("59351")
	//fixture := dns.NewFixture(solver,
	//	dns.SetResolvedZone("example.com."),
	//	dns.SetManifestPath("testdata/my-custom-solver"),
	//	dns.SetDNSServer("127.0.0.1:59351"),
	//	dns.SetUseAuthoritative(false),
	//)
	//need to uncomment and  RunConformance delete runBasic and runExtended once https://github.com/cert-manager/cert-manager/pull/4835 is merged
	//fixture.RunConformance(t)
	fixture.RunBasic(t)
	fixture.RunExtended(t)

}
