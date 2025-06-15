package main

import (
	"bytes"
	"crypto/tls"
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

// --- مقادیر ثابت برای سونی ---
const (
	sonySitekey = "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	sonyPageurl = "https://acm.account.sony.com/create_account/personal?client_id=37351a12-3e6a-4544-87ff-1eaea0846de2&scope=openid%20users&mode=signup"
)

// --- مقادیر ثابت برای اینستاگرام ---
const (
	instagramBaseURL    = "https://www.instagram.com/"
	instagramCheckEmailURL = "https://www.instagram.com/api/v1/web/accounts/check_email/"
	instagramSendVerifyURL = "https://www.instagram.com/api/v1/accounts/send_verify_email/"
)

// --- توابع کمکی عمومی ---

func randString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// ============== بخش مربوط به ساخت اکانت سونی ==============
func randDOB() string {
	year := rand.Intn(2003-1985) + 1985
	month := rand.Intn(12) + 1
	day := rand.Intn(28) + 1
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

func getCaptchaID(client *http.Client, captchaAPIKey string) (string, error) {
	data := url.Values{
		"key":       {captchaAPIKey},
		"method":    {"userrecaptcha"},
		"googlekey": {sonySitekey},
		"pageurl":   {sonyPageurl},
		"json":      {"1"},
	}
	// ... (بقیه کد این تابع بدون تغییر باقی می‌ماند)
	req, err := http.NewRequest("POST", "https://2captcha.com/in.php", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("error creating 2captcha request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
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

func pollForCaptchaToken(client *http.Client, captchaAPIKey, captchaID string) (string, error) {
	// ... (کد این تابع بدون تغییر باقی می‌ماند)
	for i := 0; i < 24; i++ {
		time.Sleep(3 * time.Second)
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

// تابع اصلی برای اجرای فرآیند سونی
func runSonyProcess(client *http.Client) {
	fmt.Println("Initializing Sony session and getting cookies...")
	initResp, err := client.Get(sonyPageurl)
	if err != nil {
		fmt.Println("Error making initial request to get cookies:", err)
		return
	}
	initResp.Body.Close()
	fmt.Println("Session initialized successfully.")

	var email, captchaAPIKey string
	fmt.Print("Enter your email for Sony account: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)
	fmt.Print("Enter your 2captcha API key: ")
	fmt.Scanln(&captchaAPIKey)
	captchaAPIKey = strings.TrimSpace(captchaAPIKey)

	fmt.Println("Solving captcha... please wait (may take up to 2 minutes)")
	captchaID, err := getCaptchaID(client, captchaAPIKey)
	if err != nil {
		fmt.Println("Error during captcha request phase:", err)
		return
	}
	fmt.Println("Captcha request sent successfully, ID:", captchaID)

	captchaToken, err := pollForCaptchaToken(client, captchaAPIKey, captchaID)
	if err != nil {
		fmt.Println("Error during captcha token retrieval phase:", err)
		return
	}
	fmt.Println("Captcha solved! Submitting registration request...")

	payload := map[string]interface{}{
		"email":              email,
		"password":           randString(10) + "aA1",
		"legalCountry":       "US",
		"language":           "en-US",
		"dateOfBirth":        randDOB(),
		"firstName":          randString(7),
		"lastName":           randString(7),
		"captchaProvider":    "google:recaptcha-invisible",
		"captchaSiteKey":     sonySitekey,
		"captchaResponse":    captchaToken,
		"clientID":           "37351a12-3e6a-4544-87ff-1eaea0846de2",
		"hashedTosPPVersion": "d3-7b2e7bfa9efbdd9371db8029cb263705",
		// ... (بقیه فیلدهای payload بدون تغییر)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error creating JSON payload:", err)
		return
	}

	req, err := http.NewRequest("POST", "https://acm.account.sony.com/api/accountInterimRegister", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://acm.account.sony.com")
	req.Header.Set("Referer", sonyPageurl)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending registration request:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("\n----- Sony Server Response -----")
	fmt.Printf("Status Code: %d\n", resp.StatusCode)
	io.Copy(os.Stdout, resp.Body)
	fmt.Println("\n------------------------------")
}

// ============== بخش جدید مربوط به اینستاگرام ==============

// این تابع به صفحه اصلی اینستاگرام میرود تا کوکی ها و توکن های لازم را بگیرد
func getInstagramSession(client *http.Client) (string, string, error) {
	req, err := http.NewRequest("GET", instagramBaseURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to get instagram page: %w", err)
	}
	defer resp.Body.Close()

	// خواندن csrftoken از کوکی ها
	var csrfToken string
	instagramURL, _ := url.Parse(instagramBaseURL)
	for _, cookie := range client.Jar.Cookies(instagramURL) {
		if cookie.Name == "csrftoken" {
			csrfToken = cookie.Value
			break
		}
	}
	if csrfToken == "" {
		return "", "", fmt.Errorf("csrftoken not found in cookies")
	}

	// خواندن jazoest از بدنه HTML
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	bodyString := string(bodyBytes)
	
	// یک روش ساده برای پیدا کردن jazoest در متن HTML
	if !strings.Contains(bodyString, `"jazoest":`) {
		return "", "", fmt.Errorf("jazoest token not found in page body")
	}
	temp := strings.Split(bodyString, `"jazoest":`)[1]
	jazoest := strings.Split(temp, ",")[0]
	jazoest = strings.Trim(jazoest, `"`)


	return csrfToken, jazoest, nil
}


// تابع برای بررسی موجود بودن ایمیل در اینستاگرام
func checkInstagramEmail(client *http.Client, email, csrfToken, jazoest string) {
	fmt.Println("\nChecking email:", email)

	data := url.Values{
		"email":   {email},
		"jazoest": {jazoest},
	}

	req, _ := http.NewRequest("POST", instagramCheckEmailURL, strings.NewReader(data.Encode()))
	
	// تنظیم هدرهای ضروری
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("X-Csrftoken", csrfToken)
	req.Header.Set("X-Instagram-Jazoest", jazoest)
	req.Header.Set("Referer", instagramBaseURL)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error checking email:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("--- Instagram Check Email Response ---")
	fmt.Println("Status Code:", resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Response Body:", string(body))
	fmt.Println("------------------------------------")
}


// تابع برای ارسال ایمیل تایید
func sendInstagramVerifyEmail(client *http.Client, email, csrfToken, jazoest string) {
	fmt.Println("\nSending verification email to:", email)

	// اینستاگرام یک device_id هم میخواهد که میتوانیم تصادفی بسازیم
	deviceID := "web-auth-e2e-" + randString(28)

	data := url.Values{
		"device_id": {deviceID},
		"email":     {email},
		"jazoest":   {jazoest},
	}

	req, _ := http.NewRequest("POST", instagramSendVerifyURL, strings.NewReader(data.Encode()))

	// تنظیم هدرهای ضروری
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("X-Csrftoken", csrfToken)
	req.Header.Set("X-Instagram-Jazoest", jazoest)
	req.Header.Set("Referer", instagramBaseURL)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending verification email:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("--- Instagram Send Verify Email Response ---")
	fmt.Println("Status Code:", resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	fmt.Println("Response Body:", string(body))
	fmt.Println("------------------------------------------")
}


// تابع اصلی برای اجرای فرآیند اینستاگرام
func runInstagramProcess(client *http.Client) {
	fmt.Println("Initializing Instagram session...")
	csrfToken, jazoest, err := getInstagramSession(client)
	if err != nil {
		fmt.Println("Error initializing session:", err)
		return
	}
	fmt.Printf("Session initialized successfully. CSRF Token: %s..., Jazoest: %s\n", csrfToken[:10], jazoest)

	var email string
	fmt.Print("\nEnter email to check on Instagram: ")
	fmt.Scanln(&email)
	email = strings.TrimSpace(email)

	// مرحله ۱: بررسی ایمیل
	checkInstagramEmail(client, email, csrfToken, jazoest)
	
	// مرحله ۲: پرسش برای ارسال ایمیل تایید
	var choice string
	fmt.Print("\nDo you want to send a verification email to this address? (y/n): ")
	fmt.Scanln(&choice)
	if strings.ToLower(strings.TrimSpace(choice)) == "y" {
		sendInstagramVerifyEmail(client, email, csrfToken, jazoest)
	} else {
		fmt.Println("Operation finished.")
	}
}


// ============== تابع Main اصلی ==============

func main() {
	// مقداردهی اولیه برای rand
	rand.Seed(time.Now().UnixNano())

	// ساخت http client با قابلیت مدیریت کوکی
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		ForceAttemptHTTP2: false,
		TLSClientConfig:   &tls.Config{MaxVersion: tls.VersionTLS12},
	}
	client := &http.Client{
		Transport: tr,
		Jar:       jar,
		Timeout:   30 * time.Second,
	}

	// نمایش منو به کاربر
	var choice int
	fmt.Println("Select an operation:")
	fmt.Println("1: Create Sony Account")
	fmt.Println("2: Instagram Email Tools")
	fmt.Print("Enter your choice (1 or 2): ")
	fmt.Scanln(&choice)

	switch choice {
	case 1:
		runSonyProcess(client)
	case 2:
		runInstagramProcess(client)
	default:
		fmt.Println("Invalid choice. Exiting.")
	}
}
