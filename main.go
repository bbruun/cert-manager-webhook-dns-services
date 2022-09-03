package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	//"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
)

type ApiDNS struct {
	ServiceIds []string `json:"service_ids,omitempty"`
	Zones      []struct {
		DomainID  string `json:"domain_id,omitempty"`
		Name      string `json:"name,omitempty"`
		ServiceID string `json:"service_id,omitempty"`
	} `json:"zones,omitempty"`
}

type RecordCreate struct {
	Name     string `json:"name"`
	Type     string `json:"type" default:"TXT"`
	Priority string `json:"priority,omitempty"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
}

func (r *RecordCreate) ToText() string {
	/*
		Output the info in readable JSON
	*/
	out, _ := json.MarshalIndent(r, "", "  ")
	return fmt.Sprintf("RecordCreate{}: \n%s\n\n", out)
}

type Services struct {
	Services []struct {
		ID           string `json:"id"`
		Domain       string `json:"domain"`
		Total        string `json:"total"`
		Status       string `json:"status"`
		Billingcycle string `json:"billingcycle"`
		NextDue      string `json:"next_due"`
		Category     string `json:"category"`
		CategoryURL  string `json:"category_url"`
		Name         string `json:"name"`
	} `json:"services,omitempty"`
}

type DomainInfo struct {
	ZoneListID string
	Domain_id  string
	Name       string
	Service_id string
}

func (d *DomainInfo) ToText() string {
	/*
		Output the info in readable JSON
	*/
	out, _ := json.MarshalIndent(d, "", "  ")
	return fmt.Sprintf("DomainInfo{}: \n%s\n\n", out)
}

type PostRequestCreateReponse struct {
	Success bool `json:"success"`
	Record  struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		TTL      int    `json:"ttl"`
		Priority int    `json:"priority"`
		Content  string `json:"content"`
	} `json:"record"`
	Info [][]string `json:"info"`
}

func (p *PostRequestCreateReponse) LogInfo(ch *v1alpha1.ChallengeRequest) string {
	/*
		Output as JSON in one line
	*/

	// out, _ := json.Marshal(p)
	// fmt.Printf("JSON: %s\n", out)

	var fqdn = ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1]
	return fmt.Sprintf("The TXT record '%v' has been created with content '%v'\n", fqdn, ch.Key)

}
func (p *PostRequestCreateReponse) ToText() string {
	/*
		Output the info in readable JSON
	*/
	out, _ := json.MarshalIndent(p, "", "  ")
	return fmt.Sprintf("PostRequestCreateReponse{}: \n%s\n\n", out)
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&customDNSProviderSolver{},
	)
}

type customDNSProviderSolver struct {
	client kubernetes.Clientset
}

type customDNSProviderConfig struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Host     string `json:"host"`
}

func (c *customDNSProviderSolver) Name() string {
	return "dns.services.cert-manager.io"
}

func findZoneInfo(ch *v1alpha1.ChallengeRequest) (DomainInfo, error) {
	//fmt.Printf("Searching for %+v in API zone data:\n", ch.DNSName)
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return DomainInfo{}, err
	}

	url := cfg.Host + "/dns"
	h := http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(cfg.Email, cfg.Password)

	r, _ := h.Do(req)
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err.Error())

	}

	var bodyApiDNS ApiDNS
	json.Unmarshal(body, &bodyApiDNS)

	var domainInformation DomainInfo
	for k, v := range bodyApiDNS.Zones {
		if strings.Contains(ch.ResolvedFQDN, v.Name) {
			domainInformation.ZoneListID = strconv.Itoa(k)
			domainInformation.Domain_id = v.DomainID
			domainInformation.Name = v.Name
			domainInformation.Service_id = v.ServiceID
			break
		}
	}

	return domainInformation, nil

}

func createTXTRecord(ch *v1alpha1.ChallengeRequest, domainInformation DomainInfo) (PostRequestCreateReponse, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return PostRequestCreateReponse{}, err
	}

	postData := RecordCreate{
		Name:    ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1],
		Type:    "TXT",
		Content: ch.Key,
		TTL:     10,
	}

	url := fmt.Sprintf("%s/service/%s/dns/%s/records", cfg.Host, domainInformation.Service_id, domainInformation.Domain_id)

	createHttpClient := http.Client{}

	var buf bytes.Buffer

	err = json.NewEncoder(&buf).Encode(postData)
	if err != nil {
		log.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(cfg.Email, cfg.Password)

	r, _ := createHttpClient.Do(req)
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err.Error())
	}

	var bodyPostData PostRequestCreateReponse
	json.Unmarshal(body, &bodyPostData)
	fmt.Printf("Response from API: %+v\n", bodyPostData)

	return bodyPostData, err
}
func getRecord(ch *v1alpha1.ChallengeRequest, domainInformation DomainInfo) (string, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		fmt.Printf("- getRecord(): could not load cfg\n")
		return "-1", err
	}

	url := fmt.Sprintf("%s/service/%s/dns/%s", cfg.Host, domainInformation.Service_id, domainInformation.Domain_id)
	var buf bytes.Buffer
	getHttpClient := http.Client{}

	req, _ := http.NewRequest(http.MethodGet, url, &buf)
	req.Header.Set("Accpet", "application/json")
	req.SetBasicAuth(cfg.Email, cfg.Password)

	r, _ := getHttpClient.Do(req)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err.Error())
	}

	type DNSList struct {
		ServiceID int    `json:"service_id,omitempty"`
		Name      string `json:"name,omitempty"`
		Records   []struct {
			ID       string `json:"id,omitempty"`
			Name     string `json:"name,omitempty"`
			TTL      int    `json:"ttl,omitempty"`
			Priority int    `json:"priority,omitempty"`
			Content  string `json:"content,omitempty"`
			Type     string `json:"type,omitempty"`
		} `json:"records,omitempty"`
	}

	var bodyPostData DNSList
	var returnId = "-1"
	json.Unmarshal(body, &bodyPostData)

	domToFind := ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1]
	for _, v := range bodyPostData.Records {
		if v.Name != domToFind {
			continue
		}
		if strings.Trim(v.Type, "\"") != "TXT" {
			continue
		}
		if strings.Trim(v.Content, "\"") != ch.Key {
			continue
		}

		returnId = v.ID
	}
	fmt.Printf("Didn't find %s as a TXT record - nothing to delete\n", ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1])
	return returnId, nil
}

func (c *customDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	ch.DNSName = os.Getenv("TEST_ZONE_NAME")

	domainInformation, err := findZoneInfo(ch)
	if err != nil {
		log.Fatal("API did not find any zones to work with.")
	}

	createdTXTRecord, err := createTXTRecord(ch, domainInformation)
	if err != nil {
		log.Fatalf("API could not create TXT record '%s' with value '%s'\n", domainInformation.Name, ch.Key)
	}
	fmt.Printf("createdTXTRecord.Record.Name = %+v\n", createdTXTRecord.Record.Name)
	if createdTXTRecord.Record.Name != "" {
		fmt.Printf("%s\n", createdTXTRecord.LogInfo(ch))
	} else {
		fmt.Printf("The TXT record was not created\n")
	}

	return nil
}

func (c *customDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	fmt.Printf("\nDelete TXT record \"%v\"\n", ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	ch.DNSName = os.Getenv("TEST_ZONE_NAME")

	domainInformation, err2 := findZoneInfo(ch)
	if err2 != nil {
		log.Fatal("Could not get zone information from API")
	}

	recordId, err := getRecord(ch, domainInformation)
	if err != nil {
		panic(err.Error())
	}

	if recordId == "-1" {
		fmt.Printf("- no apparent record found to deleted... %s\n", recordId)
		return nil
	} else {
		fmt.Printf("Record ID to delete: %s\n", recordId)
	}

	url := fmt.Sprintf("%s/service/%s/dns/%s/records/%s", cfg.Host, domainInformation.Service_id, domainInformation.Domain_id, recordId)
	var buf bytes.Buffer
	getHttpClient := http.Client{}

	req, _ := http.NewRequest(http.MethodDelete, url, &buf)
	req.Header.Set("Accpet", "application/json")
	req.SetBasicAuth(cfg.Email, cfg.Password)

	r, err := getHttpClient.Do(req)
	if err != nil {
		panic(err.Error())
	}
	if r.StatusCode != 200 {
		fmt.Printf("delete http status: %v %v\n", r.Status, r.StatusCode)
	}

	fmt.Printf("%v TXT record with ID %s has been deleted\n", ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1], recordId)

	return nil
}

func (c *customDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	// UNCOMMENT THE BELOW CODE TO MAKE A KUBERNETES CLIENTSET AVAILABLE TO
	// YOUR CUSTOM DNS PROVIDER

	//cl, err := kubernetes.NewForConfig(kubeClientConfig)
	//if err != nil {
	//	return err
	//}
	//
	//c.client = cl

	// END OF CODE TO MAKE KUBERNETES CLIENTSET AVAILABLE
	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (customDNSProviderConfig, error) {
	cfg := customDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}
