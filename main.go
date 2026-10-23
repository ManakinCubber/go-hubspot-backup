package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

func main() {
	hapikey := getHapikey()

	if isDocker() == true {
		startBackup(hapikey)
	} else {
		// give user a chance to change Hubspot account
		if getAccountInfo(hapikey) == true {
			key := answerQuestion("Press \033[32;1mENTER\033[0m to start backup\n")

			// []byte{13} = "enter" key
			if key == "" || bytes.Equal([]byte(key), []byte{13}) {
				startBackup(hapikey)
			} else if strings.ToLower(key) == "change" {
				hapikey = answerQuestion("\033[33;1mPlease enter Hubspot API key: \033[0m")
				getAccountInfo(hapikey)
			}
		}
	}

	switch runtime.GOOS {
	case "windows":
		color.Green("HUBSPOT BACKUP COMPLETE")
	default:
		fmt.Printf("\033[32;1mHUBSPOT BACKUP COMPLETE\033[0m\n")
	}
	if isDocker() == false {
		answerQuestion("Press ENTER to close.")
	}
	if failed {
		os.Exit(1)
	}
	return
}

var failed bool

// HubSpot rate limits private apps per 10s window: wait and retry instead of
// silently dropping the rest of the endpoint.
func doRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests || attempt >= 10 {
			return resp, err
		}
		resp.Body.Close()
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
}

type HubspotAccount struct {
	PortalId              int    `json:"portalId"`
	TimeZone              string `json:"timeZone"`
	Currency              string `json:"currency"`
	UtcOffsetMilliseconds int    `json:"utcOffsetMilliseconds"`
	UtcOffset             string `json:"utcOffset"`
}

type Error struct {
	Message string `json:"message"`
}

func getHapikey() string {
	var hapikey string
	// command line flags
	flag_hapikey := flag.String("hapikey", "", "Hubspot API key")
	flag.Parse()

	// if hapikey in arguments, use it, else use env variable
	if *flag_hapikey != "" {
		hapikey = *flag_hapikey
	} else if os.Getenv("HAPIKEY") != "" {
		hapikey = os.Getenv("HAPIKEY")
	} else if isDocker() == true && os.Getenv("HAPIKEY") == "" {
		fmt.Println("\033[31;1mError: No HAPIKEY present\033[0m")
		os.Exit(1)
	} else {
		// ask user for hapikey
		switch runtime.GOOS {
		case "windows":
			color.White("\033[33;1mThank you for using Hubspot Data & Content Backup! For more information and help visit https://github.com/patrykkalinowski/go-hubspot-backup \033[0m \n")
			color.Yellow("\033[33;1mThis app needs Hubspot API key to work. Learn how to get your API key here: https://developers.hubspot.com/docs/guides/apps/private-apps/overview \033[0m \n")
		default:
			fmt.Printf("\033[97;1mThank you for using Hubspot Data & Content Backup! For more information and help visit https://github.com/patrykkalinowski/go-hubspot-backup \033[0m \n")
			fmt.Printf("\033[33;1mThis app needs Hubspot API key to work. Learn how to get your API key here: https://developers.hubspot.com/docs/guides/apps/private-apps/overview \033[0m \n")
		}

		hapikey = answerQuestion("\033[33;1mPlease enter Hubspot API key: \033[0m")
		// TODO: save new hapikey to config.yml file
	}

	return hapikey
}

func getAccountInfo(hapikey string) bool {
	var hubspotAccount HubspotAccount
	var error Error
	// Create the Bearer
	bearerToken := "Bearer " + strings.TrimSpace(hapikey)

	// Create an HTTP client
	client := &http.Client{}

	// Create a request
	req, err := http.NewRequest("GET", "https://api.hubapi.com/integrations/v1/me", nil)

	if err != nil {
		fmt.Println(err)
	}

	// Set the header
	req.Header = http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {bearerToken},
	}

	// Send the request
	resp, err := doRequest(client, req)

	if err != nil {
		fmt.Println(err)
	}

	body, err := io.ReadAll(resp.Body) // body as bytess
	resp.Body.Close()

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v \033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)

		return false
	} else {
		// continue
		err = json.Unmarshal(body, &hubspotAccount) // put json body response into struct

		if err != nil {
			panic(err)
		}

		switch runtime.GOOS {
		case "windows":
			color.Green("Connected to Hubspot account %v", hubspotAccount.PortalId)
		default:
			fmt.Printf("\033[32;1mConnected to Hubspot account %v \033[0m\n", hubspotAccount.PortalId)
		}

		return true
	}

}

func answerQuestion(question string) string {
	// ask user for something and return answer
	reader := bufio.NewReader(os.Stdin)

	switch runtime.GOOS {
	case "windows":
		color.Yellow(question)
	default:
		fmt.Printf(question)
	}
	text, _ := reader.ReadString('\n')
	return strings.Trim(text, " \n")
}

func startBackup(hapikey string) {
	switch runtime.GOOS {
	case "windows":
		color.Yellow("\033[32;1mBacking up your Hubspot account...\033[0m \n")
	default:
		fmt.Printf("\033[32;1mBacking up your Hubspot account...\033[0m \n")
	}
	// https://www.sohamkamani.com/blog/2017/10/18/parsing-json-in-golang/#unstructured-data-decoding-json-to-maps
	// https://astaxie.gitbooks.io/build-web-application-with-golang/en/07.2.html

	backupHasMore(hapikey, "https://api.hubapi.com/contacts/v1/lists", "lists", 0)
	backupOnce(hapikey, "https://api.hubapi.com/content/api/v2/blogs", "blogs", 0)
	backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/blog-posts", "blog-posts", 0)
	backupLimit(hapikey, "https://api.hubapi.com/blogs/v3/blog-authors", "blog-authors", 0)
	backupLimit(hapikey, "https://api.hubapi.com/blogs/v3/topics", "blog-topics", 0)
	backupLimit(hapikey, "https://api.hubapi.com/comments/v3/comments", "blog-comments", 0)
	backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/layouts", "layouts", 0)
	backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/pages", "pages", 0)
	backupOnce(hapikey, "https://api.hubapi.com/hubdb/api/v2/tables", "hubdb-tables", 0)
	backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/templates", "templates", 0)
	backupLimit(hapikey, "https://api.hubapi.com/url-mappings/v3/url-mappings", "url-mappings", 0)
	backupHasMore(hapikey, "https://api.hubapi.com/deals/v1/deal/paged", "deals", 0)
	backupOnce(hapikey, "https://api.hubapi.com/automation/v3/workflows", "workflows", 0)
	backupHasMore(hapikey, "https://api.hubapi.com/companies/v2/companies/paged", "companies", 0)
	backupContacts(hapikey, "https://api.hubapi.com/contacts/v1/lists/all/contacts/all", "contacts", 0)
	// backupLimit(hapikey, "https://api.hubapi.com/forms/v2/forms", "forms", 0) // TODO: typeArray in results, without nesting

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	if isDocker() == false {
		switch runtime.GOOS {
		case "windows":
			color.Green("\033[32;1m############\nBackup saved in %v/hubspot-backup/%v\033[0m \n", exPath, time.Now().Format("2006-01-02"))
		default:
			fmt.Printf("\033[32;1m############\nBackup saved in %v/hubspot-backup/%v\033[0m \n", exPath, time.Now().Format("2006-01-02"))
		}
	}

	return
}

// Iterate rather than recurse: each recursive call kept its page alive until the end of the endpoint.
func backupHasMore(hapikey string, url string, endpoint string, offset float64) {
	for more := true; more; {
		offset, more = backupHasMorePage(hapikey, url, endpoint, offset)
	}
}

func backupHasMorePage(hapikey string, url string, endpoint string, offset float64) (float64, bool) {
	var error Error
	var results map[string]interface{}
	// Create the Bearer
	bearerToken := "Bearer " + strings.TrimSpace(hapikey)

	// Create an HTTP client
	client := &http.Client{}

	// Create a request
	req, err := http.NewRequest("GET", strings.TrimSpace(url+"?count=250&offset="+strconv.Itoa(int(offset))), nil)

	if err != nil {
		fmt.Println(err)
	}

	// Set the header
	req.Header = http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {bearerToken},
	}

	// Send the request
	resp, err := doRequest(client, req)

	if err != nil {
		fmt.Println(err)
	}

	// Pagination recurses once per page: a deferred Close keeps every response open until the whole endpoint is done.
	body, err := io.ReadAll(resp.Body) // body as bytes
	resp.Body.Close()

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v on %v\033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode), endpoint)
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)
		failed = true

		return 0, false
	} else {
		// continue
		err = json.Unmarshal(body, &results) // put json body response into map of strings to empty interfaces

		if err != nil {
			panic(err)
		}

		var folderpath string

		if isDocker() == true {
			// create folder
			folderpath = "/hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		} else {
			// create folder
			folderpath = "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		}
		os.MkdirAll(folderpath, 0700)

		// get items from response
		var typeArray []interface{}

		// sometimes results are within "objects" field and sometimes within endpoint name
		if results["objects"] != nil {
			typeArray = results["objects"].([]interface{})
		} else if results[endpoint] != nil {
			typeArray = results[endpoint].([]interface{})
		}
		if len(typeArray) == 0 {
			// finish if went through all records
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
			return 0, false
		}

		switch runtime.GOOS {
		case "windows":
			color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
		default:
			fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
		}

		writePage(folderpath, offset, typeArray)
		// rerun function if there are more results
		has_more := results["has-more"]
		if has_more != false {
			new_offset := results["offset"]
			return new_offset.(float64), true
		} else {
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
		}
	}
	return 0, false
}

func backupOnce(hapikey string, url string, endpoint string, offset float64) {
	var error Error
	var results map[string]interface{}
	// Create the Bearer
	bearerToken := "Bearer " + strings.TrimSpace(hapikey)

	// Create an HTTP client
	client := &http.Client{}

	// Create a request
	req, err := http.NewRequest("GET", strings.TrimSpace(url+"?count=250&offset="+strconv.Itoa(int(offset))), nil)

	if err != nil {
		fmt.Println(err)
	}

	// Set the header
	req.Header = http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {bearerToken},
	}

	// Send the request
	resp, err := doRequest(client, req)

	if err != nil {
		fmt.Println(err)
	}

	body, err := io.ReadAll(resp.Body) // body as bytes
	resp.Body.Close()

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v on %v\033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode), endpoint)
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)
		failed = true

		return
	} else {
		// continue
		err = json.Unmarshal(body, &results) // put json body response into map of strings to empty interfaces

		if err != nil {
			panic(err)
		}
		switch runtime.GOOS {
		case "windows":
			color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, int(offset))
		default:
			fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, int(offset))
		}

		var folderpath string

		if isDocker() == true {
			// create folder
			folderpath = "/hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		} else {
			// create folder
			folderpath = "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		}
		os.MkdirAll(folderpath, 0700)

		// get items from response
		var typeArray []interface{}

		// sometimes results are within "objects" field and sometimes within endpoint name
		if results["objects"] != nil {
			typeArray = results["objects"].([]interface{})
		} else if results[endpoint] != nil {
			typeArray = results[endpoint].([]interface{})
		}
		if len(typeArray) == 0 {
			// finish if went through all records
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
			return
		}
		switch runtime.GOOS {
		case "windows":
			color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
		default:
			fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
		}

		writePage(folderpath, offset, typeArray)
		switch runtime.GOOS {
		case "windows":
			color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
		default:
			fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
		}
	}
	return
}

func backupLimit(hapikey string, url string, endpoint string, offset float64) {
	for more := true; more; {
		offset, more = backupLimitPage(hapikey, url, endpoint, offset)
	}
}

func backupLimitPage(hapikey string, url string, endpoint string, offset float64) (float64, bool) {
	var error Error
	var results map[string]interface{}
	// Create the Bearer
	bearerToken := "Bearer " + strings.TrimSpace(hapikey)

	// Create an HTTP client
	client := &http.Client{}

	// Create a request
	req, err := http.NewRequest("GET", strings.TrimSpace(url+"?limit=250&offset="+strconv.Itoa(int(offset))), nil)

	if err != nil {
		fmt.Println(err)
	}

	// Set the header
	req.Header = http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {bearerToken},
	}

	// Send the request
	resp, err := doRequest(client, req)

	if err != nil {
		fmt.Println(err)
	}

	body, err := io.ReadAll(resp.Body) // body as bytes
	resp.Body.Close()

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v on %v\033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode), endpoint)
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)
		failed = true

		return 0, false
	} else {
		// continue
		err = json.Unmarshal(body, &results) // put json body response into map of strings to empty interfaces

		if err != nil {
			panic(err)
		}

		switch runtime.GOOS {
		case "windows":
			color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, int(offset))
		default:
			fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, int(offset))
		}

		var folderpath string

		if isDocker() == true {
			// create folder
			folderpath = "/hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		} else {
			// create folder
			folderpath = "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		}
		os.MkdirAll(folderpath, 0700)

		// get items from response
		var typeArray []interface{}

		// sometimes results are within "objects" field and sometimes within endpoint name
		if results["objects"] != nil {
			typeArray = results["objects"].([]interface{})
		} else if results[endpoint] != nil {
			typeArray = results[endpoint].([]interface{})
		}
		if len(typeArray) == 0 {
			// finish if went through all records
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
			return 0, false
		}

		writePage(folderpath, offset, typeArray)
		if len(typeArray) == 0 {
			// finish if went through all records
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
			return 0, false
		} else {
			switch runtime.GOOS {
			case "windows":
				color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
			default:
				fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
			}
			// run again to save next batch
			return float64(len(typeArray)) + offset, true
		}
	}
	return 0, false
}

func backupContacts(hapikey string, url string, endpoint string, offset float64) {
	for more := true; more; {
		offset, more = backupContactsPage(hapikey, url, endpoint, offset)
	}
}

func backupContactsPage(hapikey string, url string, endpoint string, offset float64) (float64, bool) {
	var error Error
	var results map[string]interface{}
	// Create the Bearer
	bearerToken := "Bearer " + strings.TrimSpace(hapikey)

	// Create an HTTP client
	client := &http.Client{}

	// Create a request
	req, err := http.NewRequest("GET", strings.TrimSpace(url+"?count=100&vidOffset="+strconv.Itoa(int(offset))), nil)

	if err != nil {
		fmt.Println(err)
	}

	// Set the header
	req.Header = http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {bearerToken},
	}

	// Send the request
	resp, err := doRequest(client, req)

	if err != nil {
		fmt.Println(err)
	}

	body, err := io.ReadAll(resp.Body) // body as bytes
	resp.Body.Close()

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v on %v\033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode), endpoint)
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)
		failed = true

		return 0, false
	} else {
		// continue
		err = json.Unmarshal(body, &results) // put json body response into map of strings to empty interfaces

		if err != nil {
			panic(err)
		}

		var folderpath string

		if isDocker() == true {
			// create folder
			folderpath = "/hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		} else {
			// create folder
			folderpath = "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
		}
		os.MkdirAll(folderpath, 0700)

		// get items from response
		var typeArray []interface{}

		// sometimes results are within "objects" field and sometimes within endpoint name
		if results["objects"] != nil {
			typeArray = results["objects"].([]interface{})
		} else if results[endpoint] != nil {
			typeArray = results[endpoint].([]interface{})
		}
		if len(typeArray) == 0 {
			// finish if went through all records
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
			return 0, false
		}

		switch runtime.GOOS {
		case "windows":
			color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
		default:
			fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
		}

		writePage(folderpath, offset, typeArray)
		// rerun function if there are more results
		has_more := results["has-more"]
		if has_more != false {
			new_offset := results["vid-offset"]
			time.Sleep(1 * time.Second)
			return new_offset.(float64), true
		} else {
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
		}
	}
	return 0, false
}

// One file per page rather than per item: hundreds of thousands of small files exhaust the container memory (kernel inode cache).
func writePage(folderpath string, offset float64, items []interface{}) {
	data, err := json.Marshal(items)
	if err != nil {
		fmt.Println(err)
		failed = true
		return
	}
	filepath := folderpath + "/" + strconv.Itoa(int(offset)) + ".json"
	if err := os.WriteFile(filepath, data, 0600); err != nil {
		fmt.Printf("failed writing to file: %s\n", err)
		failed = true
	}
}

func isDocker() bool {
	// Containerd (Kubernetes) does not create /.dockerenv.
	if os.Getenv("HUBSPOT_BACKUP_UNATTENDED") != "" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil { // check if file /.dockerenv is present
		return true
	} else {
		return false
	}
}
