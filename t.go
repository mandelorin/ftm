package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	sitekey = "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	pageurl = "https://acm.account.sony.com/create_account/personal?client_id=37351a12-3e6a-4544-87ff-1eaea0846de2&scope=openid%20users&mode=signup"
)

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

// مرحله 1: درخواست حل کپچا از 2captcha و گرفتن captchaID
func getCaptchaID(captchaAPIKey string) (string, error) {
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
	return result["request"].(string), nil
}

// مرحله 2: گرفتن توکن کپچا با poll کردن captchaID
func pollForCaptchaToken(captchaAPIKey, captchaID string) (string, error) {
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", captchaAPIKey, captchaID)
		res, err := http.Get(reqURL)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
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

	var email, captchaAPIKey string

	fmt.Print("Enter your email: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)

	fmt.Print("Enter your captcha API key: ")
	fmt.Scanln(&captchaAPIKey)
	captchaAPIKey = strings.TrimSpace(captchaAPIKey)

	fmt.Println("Solving captcha... please wait (may take up to 2 minutes)")
	captchaID, err := getCaptchaID(captchaAPIKey)
	if err != nil {
		fmt.Println("Captcha error:", err)
		os.Exit(1)
	}
	fmt.Println("Captcha requested, ID:", captchaID)

	captchaToken, err := pollForCaptchaToken(captchaAPIKey, captchaID)
	if err != nil {
		fmt.Println("Captcha polling error:", err)
		os.Exit(1)
	}
	fmt.Println("Captcha solved! Submitting registration request...")

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
		"captchaResponse":    captchaToken,
		"clientID":           "37351a12-3e6a-4544-87ff-1eaea0846de2",
		"hashedTosPPVersion": "d3-7b2e7bfa9efbdd9371db8029cb263705",
		"tosPPVersion":       4,
		"optIns":             []map[string]interface{}{{"opt_id": 57, "opted": false}},
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://acm.account.sony.com/api/accountInterimRegister", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://acm.account.sony.com")
	req.Header.Set("Referer", pageurl)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

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
}
