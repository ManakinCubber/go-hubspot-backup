package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

func main() {
	hapikey := getHapikey()

	startBackup(hapikey)

	switch runtime.GOOS {
	case "windows":
		color.Green("HUBSPOT BACKUP COMPLETE")
	default:
		fmt.Printf("\033[32;1mHUBSPOT BACKUP COMPLETE\033[0m\n")
	}
	return
}

type HubspotAccount struct {
	PortalId              int    `json:"portalId"`
	TimeZone              string `json:"timeZone"`
	Currency              string `json:"currency"`
	UtcOffsetMilliseconds int    `json:"utcOffsetMilliseconds"`
	UtcOffset             string `json:"utcOffset"`
}

type HubspotConfig struct {
	Hapikey    string `json: hapiKey`
	PrivateApp bool   `json: privateApp`
}

type Error struct {
	Message string `json:"message"`
}

func getHapikey() *HubspotConfig {
	var hapikey string
	privateApp := true

	// command line flags
	flag_hapikey := flag.String("hapikey", "", "Hubspot API key")
	flag_accesskey := flag.String("accesskey", "", "Hubspot API access key")
	flag.Parse()
	// if hapikey in arguments, use it, else use env variable
	if *flag_hapikey != "" {
		hapikey = *flag_hapikey
	} else if os.Getenv("HAPIKEY") != "" {
		hapikey = os.Getenv("HAPIKEY")
	} else if *flag_accesskey != "" {
		privateApp = true
		hapikey = *flag_accesskey
	} else if os.Getenv("HAPI_ACCESS_KEY") != "" {
		privateApp = true
		hapikey = os.Getenv("HAPI_ACCESS_KEY")
	} else {
		// ask user for hapikey
		switch runtime.GOOS {
		case "windows":
			color.White("\033[33;1m Thank you for using Hubspot Data & Content Backup! For more information and help visit https://hubspot-backup.patrykkalinowski.com \033[0m \n")
			color.Yellow("\033[33;1mThis app needs Hubspot API key to work. Learn how to get your API key here: https://knowledge.hubspot.com/Integrations/How-do-I-get-my-HubSpot-API-key \033[0m \n")
		default:
			fmt.Printf("\033[97;1m Thank you for using Hubspot Data & Content Backup! For more information and help visit https://hubspot-backup.patrykkalinowski.com \033[0m \n")
			fmt.Printf("\033[33;1mThis app needs Hubspot API key to work. Learn how to get your API key here: https://knowledge.hubspot.com/Integrations/How-do-I-get-my-HubSpot-API-key \033[0m \n")
		}

		hapikey = answerQuestion("\033[33;1mPlease enter Hubspot API key: \033[0m")
		// TODO: save new hapikey to config.yml file
	}

	hubspotConfig := &HubspotConfig{
		Hapikey:    hapikey,
		PrivateApp: privateApp,
	}

	return hubspotConfig
}

func getAccountInfo(hapikey string) bool {
	var hubspotAccount HubspotAccount
	var error Error

	// Windows CMD: https://stackoverflow.com/questions/55945325/golang-url-parse-always-return-invalid-control-character-url
	resp, err := http.Get(strings.TrimSpace("https://api.hubapi.com/integrations/v1/me" + "?hapikey=" + hapikey))
	if err != nil {
		fmt.Println(err)
	}
	body, err := ioutil.ReadAll(resp.Body) // body as bytess
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

func executeRequest(hubspotConfig *HubspotConfig, url string) (*http.Response, error) {
	// Create a new GET request
	if !hubspotConfig.PrivateApp {
		url += "&hapikey=" + hubspotConfig.Hapikey
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		return nil, err
	}

	if hubspotConfig.PrivateApp {
		req.Header.Set("Authorization", "Bearer "+hubspotConfig.Hapikey)
	}

	// Create an HTTP client
	client := &http.Client{}

	// Send the request using the client
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error making request: %v\n", err)
		return resp, err
	}

	return resp, nil
}

func startBackup(hapikey *HubspotConfig) {
	var wg sync.WaitGroup

	switch runtime.GOOS {
	case "windows":
		color.Yellow("\033[32;1mBacking up your Hubspot account...\033[0m \n")
	default:
		fmt.Printf("\033[32;1mBacking up your Hubspot account...\033[0m \n")
	}

	// https://www.sohamkamani.com/blog/2017/10/18/parsing-json-in-golang/#unstructured-data-decoding-json-to-maps
	// https://astaxie.gitbooks.io/build-web-application-with-golang/en/07.2.html
	wg.Add(16)
	go backupHasMore(hapikey, "https://api.hubapi.com/contacts/v1/lists", "lists", 0, &wg)
	go backupOnce(hapikey, "https://api.hubapi.com/content/api/v2/blogs", "blogs", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/blog-posts", "blog-posts", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/blogs/v3/blog-authors", "blog-authors", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/blogs/v3/topics", "blog-topics", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/comments/v3/comments", "blog-comments", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/layouts", "layouts", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/pages", "pages", 0, &wg)
	go backupOnce(hapikey, "https://api.hubapi.com/hubdb/api/v2/tables", "hubdb-tables", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/content/api/v2/templates", "templates", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/url-mappings/v3/url-mappings", "url-mappings", 0, &wg)
	go backupHasMore(hapikey, "https://api.hubapi.com/deals/v1/deal/paged", "deals", 0, &wg)
	go backupLimit(hapikey, "https://api.hubapi.com/marketing-emails/v1/emails", "marketing-emails", 0, &wg)
	go backupOnce(hapikey, "https://api.hubapi.com/automation/v3/workflows", "workflows", 0, &wg)
	go backupHasMore(hapikey, "https://api.hubapi.com/companies/v2/companies/paged", "companies", 0, &wg)
	go backupContacts(hapikey, "https://api.hubapi.com/contacts/v1/lists/all/contacts/all", "contacts", 0, &wg)
	//go backupLimit(hapikey, "https://api.hubapi.com/forms/v2/forms", "forms", 0, &wg) // TODO: typeArray in results, without nesting

	wg.Wait()
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	switch runtime.GOOS {
	case "windows":
		color.Green("\033[32;1m############\nBackup saved in %v/hubspot-backup/%v\033[0m \n", exPath, time.Now().Format("2006-01-02"))
	default:
		fmt.Printf("\033[32;1m############\nBackup saved in %v/hubspot-backup/%v\033[0m \n", exPath, time.Now().Format("2006-01-02"))
	}
	return
}

func backupHasMore(hubspotConfig *HubspotConfig, url string, endpoint string, offset float64, wg *sync.WaitGroup) {
	defer wg.Done()
	var error Error
	var results map[string]interface{}

	// get data from API
	resp, err := executeRequest(hubspotConfig, strings.TrimSpace(url+"?count=250&offset="+strconv.Itoa(int(offset))))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body) // body as bytes

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v \033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)

		return
	} else {
		// continue
		err = json.Unmarshal(body, &results) // put json body response into map of strings to empty interfaces

		if err != nil {
			panic(err)
		}

		// create folder
		folderpath := "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
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

		// for each item
		for k, v := range typeArray {
			itemnumber := k + int(offset)
			filepath := string(folderpath + "/" + strconv.Itoa(itemnumber) + ".json")
			// create file
			file, err := os.Create(filepath)
			if err != nil {
				fmt.Println("failed creating file: %s", err)
			}
			// create json
			json, err := json.Marshal(v)
			if err != nil {
				fmt.Println(err)
			}
			// write json to file
			file.WriteString(string(json[:]))

			if err != nil {
				fmt.Println("failed writing to file: %s", err)
			}
			file.Close()
		}

		// rerun function if there are more results
		has_more := results["has-more"]
		if has_more != false {
			new_offset := results["offset"]
			wg.Add(1)
			go backupHasMore(hubspotConfig, url, endpoint, new_offset.(float64), wg)
		} else {
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
		}
	}
	return
}

func backupOnce(hubspotConfig *HubspotConfig, url string, endpoint string, offset float64, wg *sync.WaitGroup) {
	defer wg.Done()
	var error Error
	var results map[string]interface{}

	// get data from API
	resp, err := executeRequest(hubspotConfig, strings.TrimSpace(url+"?count=250&offset="+strconv.Itoa(int(offset))))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body) // body as bytes

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v \033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)

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

		// create folder
		folderpath := "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
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

		// for each item
		for k, v := range typeArray {
			itemnumber := k + int(offset)
			filepath := string(folderpath + "/" + strconv.Itoa(itemnumber) + ".json")
			// create file
			file, err := os.Create(filepath)
			if err != nil {
				fmt.Println("failed creating file: %s", err)
			}
			// create json
			json, err := json.Marshal(v)
			if err != nil {
				fmt.Println(err)
			}
			// write json to file
			file.WriteString(string(json[:]))

			if err != nil {
				fmt.Println("failed writing to file: %s", err)
			}
			file.Close()
		}

		switch runtime.GOOS {
		case "windows":
			color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
		default:
			fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
		}
	}
	return
}

func backupLimit(hubspotConfig *HubspotConfig, url string, endpoint string, offset float64, wg *sync.WaitGroup) {
	defer wg.Done()
	var error Error
	var results map[string]interface{}

	// get data from API
	resp, err := executeRequest(hubspotConfig, strings.TrimSpace(url+"?count=250&offset="+strconv.Itoa(int(offset))))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body) // body as bytes

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v \033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)

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

		// create folder
		folderpath := "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
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

		// for each item
		for k, v := range typeArray {
			itemnumber := k + int(offset)
			filepath := string(folderpath + "/" + strconv.Itoa(itemnumber) + ".json")
			// create file
			file, err := os.Create(filepath)
			if err != nil {
				fmt.Println("failed creating file: %s", err)
			}
			// create json
			json, err := json.Marshal(v)
			if err != nil {
				fmt.Println(err)
			}
			// write json to file
			file.WriteString(string(json[:]))

			if err != nil {
				fmt.Println("failed writing to file: %s", err)
			}
			file.Close()
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
		} else {
			switch runtime.GOOS {
			case "windows":
				color.Yellow("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
			default:
				fmt.Printf("\r\033[33;1mBacking up %v: %v\033[0m", endpoint, len(typeArray)+int(offset))
			}
			// run again to save next batch
			wg.Add(1)
			go backupLimit(hubspotConfig, url, endpoint, float64(len(typeArray))+offset, wg)
		}
	}
	return
}

func backupContacts(hubspotConfig *HubspotConfig, url string, endpoint string, offset float64, wg *sync.WaitGroup) {
	defer wg.Done()
	var error Error
	var results map[string]interface{}

	// get data from API
	resp, err := executeRequest(hubspotConfig, strings.TrimSpace(url+"?count=100&vidOffset="+strconv.Itoa(int(offset))))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body) // body as bytes

	if resp.StatusCode > 299 {
		// if error
		fmt.Printf("\033[31;1mError: %v %v \033[0m\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		err = json.Unmarshal(body, &error)
		fmt.Println(error.Message)

		return
	} else {
		// continue
		err = json.Unmarshal(body, &results) // put json body response into map of strings to empty interfaces

		if err != nil {
			panic(err)
		}

		// create folder
		folderpath := "hubspot-backup/" + time.Now().Format("2006-01-02") + "/" + endpoint
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

		// for each item
		for k, v := range typeArray {
			itemnumber := k + int(offset)
			filepath := string(folderpath + "/" + strconv.Itoa(itemnumber) + ".json")
			// create file
			file, err := os.Create(filepath)
			if err != nil {
				fmt.Println("failed creating file: %s", err)
			}
			// create json
			json, err := json.Marshal(v)
			if err != nil {
				fmt.Println(err)
			}
			// write json to file
			file.WriteString(string(json[:]))

			if err != nil {
				fmt.Println("failed writing to file: %s", err)
			}
			file.Close()
		}

		// rerun function if there are more results
		has_more := results["has-more"]
		if has_more != false {
			new_offset := results["vid-offset"]
			time.Sleep(1 * time.Second)
			go backupContacts(hubspotConfig, url, endpoint, new_offset.(float64), wg)
		} else {
			switch runtime.GOOS {
			case "windows":
				color.Green("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			default:
				fmt.Printf("\n\033[32;1mBacked up all %v \033[0m\n", endpoint)
			}
		}
	}
	return
}
