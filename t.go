package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const captchaAPIKey = "1c800073c3a5c4fc84bd79762cb0e0b5" // اینجا api key معتبر 2captcha را بگذارید

func randString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func randDOB() string {
	year := rand.Intn(2003-1985) + 1985
	month := rand.Intn(12) + 1
	day := rand.Intn(28) + 1
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

func getCaptchaToken(sitekey, pageurl string) (string, error) {
	// ارسال درخواست حل کپچا به 2captcha
	resp, err := http.PostForm(
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
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	if result["status"].(float64) != 1 {
		return "", fmt.Errorf("2captcha error: %v", result["request"])
	}
	captchaID := result["request"].(string)

	// گرفتن جواب کپچا (polling)
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=1c800073c3a5c4fc84bd79762cb0e0b5&action=get&id=%s&json=1", captchaAPIKey, captchaID)
		res, err := http.Get(reqURL)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		var poll map[string]interface{}
		json.Unmarshal(body, &poll)
		if poll["status"].(float64) == 1 {
			return poll["request"].(string), nil
		}
	}
	return "", fmt.Errorf("Captcha not solved in time")
}

func main() {
	rand.Seed(time.Now().UnixNano())
	var email string

	fmt.Print("Enter your email: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)

	sitekey := "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	pageurl := "https://acm.account.sony.com/create_account/personal?client_id=37351a12-3e6a-4544-87ff-1eaea0846de2&scope=openid%20users&mode=signup"

	fmt.Println("Solving captcha... please wait (may take up to 2 minutes)")
	captchaResp, err := getCaptchaToken(sitekey, pageurl)
	if err != nil {
		fmt.Println("Captcha error:", err)
		return
	}
	fmt.Println("Captcha solved!")

	payload := map[string]interface{}{
		"email":              email,
		"password":           randString(12),
		"legalCountry":       "US",
		"language":           "en-US",
		"dateOfBirth":        randDOB(),
		"firstName":          randString(7),
		"lastName":           randString(7),
		"securityQuestion":   "Where were you born?",
		"securityAnswer":     randString(6),
		"captchaProvider":    "google:recaptcha-invisible",
		"captchaSiteKey":     sitekey,
		"captchaResponse":    captchaResp,
		"clientID":           "37351a12-3e6a-4544-87ff-1eaea0846de2",
		"hashedTosPPVersion": "d3-7b2e7bfa9efbdd9371db8029cb263705",
		"tosPPVersion":       4,
		"optIns":             []map[string]interface{}{{"opt_id": 57, "opted": false}},
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://acm.account.sony.com/api/accountInterimRegister", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	// غیرفعال کردن HTTP/2 برای جلوگیری از خطای INTERNAL_ERROR
	tr := &http.Transport{
		ForceAttemptHTTP2: false,
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("HTTP error:", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	fmt.Println("----- Registration Response -----")
	prettyResult, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(prettyResult))

	if body, ok := result["body"].(map[string]interface{}); ok {
		if resp, ok := body["response"].(map[string]interface{}); ok {
			if verificationID, ok := resp["verificationID"].(string); ok && verificationID != "" {
				fmt.Println("Your verificationID:", verificationID)
			}
		}
	}
}
