package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func main() {
	// گرفتن اطلاعات کاربر
	var (
		email, password, firstName, lastName, dob, securityA, captchaResp string
	)
	fmt.Print("Enter email: ")
	fmt.Scanln(&email)
	fmt.Print("Enter password: ")
	fmt.Scanln(&password)
	fmt.Print("Enter first name: ")
	fmt.Scanln(&firstName)
	fmt.Print("Enter last name: ")
	fmt.Scanln(&lastName)
	fmt.Print("Enter date of birth (yyyy-mm-dd): ")
	fmt.Scanln(&dob)
	fmt.Print("Enter security answer (مثلاً یک کلمه دلخواه): ")
	fmt.Scanln(&securityA)
	fmt.Print("Enter captcha token (از 2captcha): ")
	fmt.Scanln(&captchaResp)

	// مقداردهی پارامترهای ثابت
	captchaSiteKey := "6LdWbTcaAAAAADFe7Vs6-1jfzSnprQwDWJ51aRep"
	clientID := "37351a12-3e6a-4544-87ff-1eaea0846de2"
	securityQ := "Where were you born?"
	hashedTosPPVersion := "d3-7b2e7bfa9efbdd9371db8029cb263705"

	payload := map[string]interface{}{
		"email":            strings.ReplaceAll(email, "%40", "@"),
		"password":         password,
		"legalCountry":     "US",
		"language":         "en-US",
		"dateOfBirth":      dob,
		"firstName":        firstName,
		"lastName":         lastName,
		"securityQuestion": securityQ,
		"securityAnswer":   securityA,
		"captchaProvider":  "google:recaptcha-invisible",
		"captchaSiteKey":   captchaSiteKey,
		"captchaResponse":  captchaResp,
		"clientID":         clientID,
		"hashedTosPPVersion": hashedTosPPVersion,
		"tosPPVersion":     4,
		"optIns":           []map[string]interface{}{{"opt_id": 57, "opted": false}},
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://acm.account.sony.com/api/accountInterimRegister", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("HTTP error:", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	fmt.Println("----- Response from Sony -----")
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))

	// گرفتن verificationID برای مرحله بعد
	if body, ok := result["body"].(map[string]interface{}); ok {
		if resp, ok := body["response"].(map[string]interface{}); ok {
			if verificationID, ok := resp["verificationID"].(string); ok {
				fmt.Println("Your verificationID for next step (email verification):", verificationID)
				os.Exit(0)
			}
		}
	}
	fmt.Println("No verificationID found. Something went wrong.")
}
