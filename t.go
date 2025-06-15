package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	sitekey = "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	pageurl = "https://acm.account.sony.com/create_account/personal?client_id=37351a12-3e6a-4544-87ff-1eaea0846de2&scope=openid%20users&mode=signup"
)

// randString generates a random string for use in names, etc.
func randString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// randDOB generates a random date of birth between 1985 and 2003.
func randDOB() string {
	year := rand.Intn(2003-1985) + 1985
	month := rand.Intn(12) + 1
	day := rand.Intn(28) + 1
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

// getCaptchaID: Step 1 - Request captcha solution from 2captcha and get captchaID.
// This function now needs the shared http client to use the cookie jar.
func getCaptchaID(client *http.Client, captchaAPIKey string) (string, error) {
	resp, err := client.PostForm(
		"https://2captcha.com/in.php",
		url.Values{
			"key":       {captchaAPIKey},
			"method":    {"userrecaptcha"},
			"googlekey": {sitekey},
			"pageurl":   {pageurl},
			"json":      {"1"},
		},
	)
	if err != nil {
		return "", fmt.Errorf("error sending initial request to 2captcha: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response from 2captcha: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("error parsing JSON response from 2captcha: %w", err)
	}

	if status, ok := result["status"].(float64); !ok || status != 1 {
		return "", fmt.Errorf("2captcha service returned an error: %v", result["request"])
	}

	return result["request"].(string), nil
}

// pollForCaptchaToken: Step 2 - Get the captcha token by polling with the captchaID.
// This function also needs the shared client.
func pollForCaptchaToken(client *http.Client, captchaAPIKey, captchaID string) (string, error) {
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", captchaAPIKey, captchaID)
		res, err := client.Get(reqURL)
		if err != nil {
			fmt.Printf("Network error while checking captcha status: %v. Retrying...\n", err)
			continue
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			res.Body.Close()
			return "", fmt.Errorf("error reading captcha status response: %w", err)
		}
		res.Body.Close()

		var poll map[string]interface{}
		if err := json.Unmarshal(body, &poll); err != nil {
			return "", fmt.Errorf("error parsing captcha status JSON response: %w", err)
		}

		if status, ok := poll["status"].(float64); ok && status == 1 {
			return poll["request"].(string), nil
		}
	}
	return "", fmt.Errorf("captcha was not solved in the specified time")
}

func main() {
	// 1. Create a client with a cookie jar to manage cookies automatically.
	jar, err := cookiejar.New(nil)
	if err != nil {
		fmt.Println("Error creating cookie jar:", err)
		os.Exit(1)
	}

	// Force HTTP/1.1 to avoid the server's HTTP/2 INTERNAL_ERROR
	tr := &http.Transport{
		ForceAttemptHTTP2: false,
	}
	// This is the ONLY place the client is created.
	client := &http.Client{
		Transport: tr,
		Jar:       jar, // Attach the cookie jar to the client
		Timeout:   30 * time.Second,
	}

	// 2. Make an initial request to the page to get necessary cookies.
	fmt.Println("Initializing session and getting cookies...")
	initResp, err := client.Get(pageurl)
	if err != nil {
		fmt.Println("Error making initial request to get cookies:", err)
		os.Exit(1)
	}
	initResp.Body.Close() // We don't need the body, just the cookies.
	fmt.Println("Session initialized successfully.")

	var email, captchaAPIKey string

	fmt.Print("Enter your email: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)

	fmt.Print("Enter your 2captcha API key: ")
	fmt.Scanln(&captchaAPIKey)
	captchaAPIKey = strings.TrimSpace(captchaAPIKey)

	fmt.Println("Solving captcha... please wait (may take up to 2 minutes)")
	// Pass the shared client to the captcha functions
	captchaID, err := getCaptchaID(client, captchaAPIKey)
	if err != nil {
		fmt.Println("Error during captcha request phase:", err)
		os.Exit(1)
	}
	fmt.Println("Captcha request sent successfully, ID:", captchaID)

	// Pass the shared client to the captcha functions
	captchaToken, err := pollForCaptchaToken(client, captchaAPIKey, captchaID)
	if err != nil {
		fmt.Println("Error during captcha token retrieval phase:", err)
		os.Exit(1)
	}
	fmt.Println("Captcha solved! Submitting registration request...")

	payload := map[string]interface{}{
		"email":                   email,
		"password":                randString(10) + "aA1",
		"legalCountry":            "US",
		"language":                "en-US",
		"dateOfBirth":             randDOB(),
		"firstName":               randString(7),
		"lastName":                randString(7),
		"securityQuestion":        "Where were you born?",
		"securityAnswer":          randString(6),
		"captchaProvider":         "google:recaptcha-invisible",
		"captchaSiteKey":          sitekey,
		"captchaResponse":         captchaToken,
		"clientID":                "37351a12-3e6a-4544-87ff-1eaea0846de2",
		"hashedTosPPVersion":      "d3-7b2e7bfa9efbdd9371db8029cb263705",
		"tosPPVersion":            4,
		"optIns":                  []map[string]interface{}{{"opt_id": 57, "opted": false}},
		"address1":                "",
		"address2":                "",
		"address3":                "",
		"city":                    "",
		"state":                   "",
		"postalCode":              "",
		"captchaChallenge":        "",
		"firstNamePhoneticValue":  "",
		"familyNamePhoneticValue": "",
		"formNumber":              nil,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error creating JSON payload:", err)
		os.Exit(1)
	}

	req, err := http.NewRequest("POST", "https://acm.account.sony.com/api/accountInterimRegister", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://acm.account.sony.com")
	req.Header.Set("Referer", pageurl)

	// The second, duplicate client definition has been DELETED from here.
	// We use the client that was defined at the top of main().
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending registration request:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("\n----- Sony Server Response -----")
	fmt.Printf("Status Code: %d\n", resp.StatusCode)

	var result map[string]interface{}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		fmt.Println("Received response was not in JSON format or could not be read.")
		fmt.Println("Response body:", string(bodyBytes))
		return
	}

	prettyResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Println("Error pretty-printing JSON response:", err)
		return
	}
	fmt.Println(string(prettyResult))
}
