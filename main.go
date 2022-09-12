package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	cmmeta "github.com/jetstack/cert-manager/pkg/apis/meta/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"k8s.io/klog"

	//"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
)

// Password string `json:"password"`

// PodNamespace is the namespace of the webhook pod
var PodNamespace = os.Getenv("POD_NAMESPACE")

// PodSecretName is the name of the secret to obtain the Linode API token from
var PodSecretName = os.Getenv("POD_SECRET_NAME")

// PodSecretKey is the key of the Linode API token within the secret POD_SECRET_NAME
var PodSecretKey = os.Getenv("POD_SECRET_KEY")

type dnsServicesProviderConfig struct {
	UsernameSecretRef cmmeta.SecretKeySelector `json:"usernameSecretRef"`
	PasswordSecretRef cmmeta.SecretKeySelector `json:"passwordSecretRef"`
	Username          string                   `json:",omitempty"`
	Password          string                   `json:",omitempty"`
}

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
	TTL      string `json:"ttl"`
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
	return fmt.Sprintf("PostRequestCreateReponse{}: \n%s\n", out)
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&dnsServicesProviderSolver{},
	)
}

type dnsServicesProviderSolver struct {
	//client kubernetes.Clientset
	client *kubernetes.Clientset
	// ctx    context.Context
}

func (c *dnsServicesProviderSolver) Name() string {
	return "dns.services.cert-manager.io"
}

func (c *dnsServicesProviderSolver) findZoneInfo(ch *v1alpha1.ChallengeRequest, cfg dnsServicesProviderConfig) (DomainInfo, error) {
	fmt.Printf("Searching for %+v in API zone data:\n", ch.DNSName)

	if cfg.Username == "" || cfg.Password == "" {
		log.Fatalf("no username or password provided or found\n")
	}

	url := "https://dns.services/api/dns"
	fmt.Printf("- url: %s\n", url)
	if cfg.Password != "" {
		fmt.Printf("- credentials: %s:%s\n", cfg.Username, "*******")
	} else {
		fmt.Printf("- credentials is missing password !!!")
		err := errors.New("username or password missing please check Secret")
		return DomainInfo{}, err
	}
	h := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("could not create new http requestion object\n%v\n", err)
		return DomainInfo{}, err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)

	r, err := h.Do(req)
	if err != nil {
		fmt.Printf("Error in connecting to API\n%v\n", err)
		return DomainInfo{}, err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("could not decipher restult from API\n%v\n", err)
		return DomainInfo{}, err
	}

	var bodyApiDNS ApiDNS
	json.Unmarshal(body, &bodyApiDNS)

	var domainInformation DomainInfo
	for k, v := range bodyApiDNS.Zones {
		fmt.Printf("- bodyApiDNS response: %+v\n", v)
		fmt.Printf("- ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1]: %s\n", ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1])
		if strings.Contains(ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1], v.Name) {
			domainInformation.ZoneListID = strconv.Itoa(k)
			domainInformation.Domain_id = v.DomainID
			domainInformation.Name = v.Name
			domainInformation.Service_id = v.ServiceID
			break
		}
	}

	return domainInformation, nil

}

func (c *dnsServicesProviderSolver) createTXTRecord(ch *v1alpha1.ChallengeRequest, domainInformation DomainInfo, cfg dnsServicesProviderConfig) (PostRequestCreateReponse, error) {
	fmt.Printf("createTXTRecord()\n")
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return PostRequestCreateReponse{}, err
	}

	// Convert Secret key data to string
	tmpSecret, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), cfg.PasswordSecretRef.Name, metav1.GetOptions{})
	cfg.Username = string(tmpSecret.Data[cfg.UsernameSecretRef.Key])
	cfg.Password = string(tmpSecret.Data[cfg.PasswordSecretRef.Key])
	if err != nil {
		fmt.Printf("- error : %+v\n", err)
	}

	postData := RecordCreate{
		Name:     ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1],
		Type:     "TXT",
		Content:  ch.Key,
		TTL:      "10",
		Priority: "10",
	}

	url := fmt.Sprintf("https://dns.services/api/service/%s/dns/%s/records", domainInformation.Service_id, domainInformation.Domain_id)
	createHttpClient := http.Client{}

	var buf bytes.Buffer

	err = json.NewEncoder(&buf).Encode(postData)
	if err != nil {
		log.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(cfg.Username, cfg.Password)

	r, err := createHttpClient.Do(req)
	if err != nil {
		fmt.Printf("Error in connecting to API\n%v\n", err)
		return PostRequestCreateReponse{}, err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err.Error())
	}

	var bodyPostData PostRequestCreateReponse
	json.Unmarshal(body, &bodyPostData)
	fmt.Printf("- response from API: %+v\n", bodyPostData)

	return bodyPostData, err
}

func (c *dnsServicesProviderSolver) getRecord(ch *v1alpha1.ChallengeRequest, domainInformation DomainInfo, cfg dnsServicesProviderConfig) (string, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return "-1", err
	}

	// Convert Secret key data to string
	tmpSecret, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), cfg.PasswordSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("- error : %+v\n", err)
	}
	cfg.Username = string(tmpSecret.Data[cfg.UsernameSecretRef.Key])
	cfg.Password = string(tmpSecret.Data[cfg.PasswordSecretRef.Key])

	url := fmt.Sprintf("https://dns.services/api/service/%s/dns/%s", domainInformation.Service_id, domainInformation.Domain_id)
	var buf bytes.Buffer
	getHttpClient := http.Client{}

	req, _ := http.NewRequest(http.MethodGet, url, &buf)
	req.SetBasicAuth(cfg.Username, cfg.Password)

	r, _ := getHttpClient.Do(req)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err.Error())
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
	fmt.Printf("- didn't find %s as a TXT record - nothing to delete\n", ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1])
	return returnId, nil
}

func (c *dnsServicesProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	ch.DNSName = os.Getenv("TEST_ZONE_NAME")

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		fmt.Printf("- an error occurred loading configuration: %s\n", err)
		return err
	}

	// Convert Secret key data to string
	tmpSecret, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), cfg.PasswordSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("- error : %+v\n", err)
	}
	cfg.Username = string(tmpSecret.Data[cfg.UsernameSecretRef.Key])
	cfg.Password = string(tmpSecret.Data[cfg.PasswordSecretRef.Key])

	domainInformation, err := c.findZoneInfo(ch, cfg)
	if err != nil {
		log.Fatal("- did not find any zones to work with in the API.")
	}

	createdTXTRecord, err := c.createTXTRecord(ch, domainInformation, cfg)
	if err != nil {
		log.Fatalf("- could not create TXT record '%s' with value '%s'\nError: %+v", domainInformation.Name, ch.Key, err)
	}

	if createdTXTRecord.Record.Name != "" {
		fmt.Printf("%s\n", createdTXTRecord.LogInfo(ch))
	} else {
		fmt.Printf("- the TXT record '%s' was not created\n", ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1])
	}

	return nil
}

func (c *dnsServicesProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	fmt.Printf("\nDelete TXT record \"%v\"\n", ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	// Load configuration data
	tmpSecret, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), cfg.PasswordSecretRef.Name, metav1.GetOptions{})
	cfg.Username = string(tmpSecret.Data[cfg.UsernameSecretRef.Key])
	cfg.Password = string(tmpSecret.Data[cfg.PasswordSecretRef.Key])
	if err != nil {
		fmt.Printf("- error : %+v\n", err)
	}

	ch.DNSName = os.Getenv("TEST_ZONE_NAME")

	domainInformation, err2 := c.findZoneInfo(ch, cfg)
	if err2 != nil {
		log.Fatal("Could not get zone information from API")
	}

	recordId, err := c.getRecord(ch, domainInformation, cfg)
	if err != nil {
		panic(err.Error())
	}

	if recordId == "-1" {
		fmt.Printf("- no apparent record found to deleted...\n")
		return nil
	} else {
		fmt.Printf("- record ID to delete: %s\n", recordId)
	}

	url := fmt.Sprintf("https://dns.services/api/service/%s/dns/%s/records/%s", domainInformation.Service_id, domainInformation.Domain_id, recordId)
	var buf bytes.Buffer
	getHttpClient := http.Client{}

	req, _ := http.NewRequest(http.MethodDelete, url, &buf)
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(cfg.Username, cfg.Password)

	r, err := getHttpClient.Do(req)
	if err != nil {
		panic(err.Error())
	}
	if r.StatusCode != 200 {
		fmt.Printf("- delete http status: %v %v\n", r.Status, r.StatusCode)
	}

	fmt.Printf("- %v TXT record with ID %s has been deleted\n", ch.ResolvedFQDN[:len(ch.ResolvedFQDN)-1], recordId)

	return nil
}

func (c *dnsServicesProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = cl

	// END OF CODE TO MAKE KUBERNETES CLIENTSET AVAILABLE
	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (dnsServicesProviderConfig, error) {
	cfg := dnsServicesProviderConfig{}

	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}
