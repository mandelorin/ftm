package main

import (
	"fmt"
	"net/http"
	"regexp"
	"io/ioutil"
)

func main() {
	url := "https://acm.account.sony.com/create_account/account_info?client_id=37351a12-3e6a-4544-87ff-1eaea0846de2&redirect_uri=https%3A%2F%2Felectronics.sony.com%3FauthRedirect%3Dtrue&nonce=379a1503-ab4b-4cb8-b7e2-29fdd8f56b9a&state=fcc72b7a-a317-4cc9-b230-42f92ec39236&scope=openid%20users#page-top"

	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("HTTP error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	re := regexp.MustCompile(`data-sitekey="([a-zA-Z0-9\-_]+)"`)
	match := re.FindStringSubmatch(string(body))
	if len(match) > 1 {
		fmt.Println("sitekey found:", match[1])
	} else {
		fmt.Println("sitekey not found")
	}
}
