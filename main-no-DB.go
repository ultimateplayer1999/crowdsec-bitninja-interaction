package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
        //"time"
        //"net/http"
        //"bytes"
)

func handleAdd(ip string, duration string, reason string, jsonObject map[string]interface{}) {
	cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--add=%s", ip), fmt.Sprintf("--comment=%s", reason))
	out, err := cmd.Output()
	if err != nil {
		fmt.Println("Error adding IP:", err)
		return
	}
	fmt.Println(string(out))
        //webhookURL := "https://discord.com/api/webhooks/1215603532451946566/JsMb7nSHAHOIZ4FJZf8xu0I495dsh1_5LV5x7x52KQBfYM5BoamNCDPAiRbk7yv6q-cX"
        // embedColor := 0xff0000
        //hostname, err := os.Hostname()
        //if err != nil {
        //    fmt.Println("Error:", err)
        //}

        //currentDate := time.Now().Format("2006-01-02 15:04")

        // Create the embed
        // embed := map[string]interface{} {
        //    "Banned IP":   ip,
        //    "Comment":     reason,
        //    "Duration":    duration,
        //    "JSON_DATA":   jsonObject,
        //}

        //payload := map[string]interface{} {
            //"content": fmt.Sprintf("Banned IP: %s\nComment: %s\nDuration: %s\nJSON_DATA: %s", ip, reason, duration, jsonObject),
        //   "embeds": []map[string]interface{}{embed},
        //}
        //payload := map[string]string {
        //   "content": fmt.Sprintf("Hostname: %s\nCurrent date: %s\nBanned IP: %s\nComment: %s\nDuration: %s\nJSON_DATA: %s", hostname, currentDate, ip, reason, duration, jsonObject),
        //}
        //jsonData, err := json.Marshal(payload)
        //if err != nil {
        //    fmt.Println("Error marshaling JSON:", err)
        //    return
        //}
        // Create the HTTP request
        //req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
        //if err != nil {
        //    fmt.Println("Error creating request:", err)
        //    return
        //}
        //req.Header.Set("Content-Type", "application/json")

        // Send the HTTP request
        //client := &http.Client{}
        //resp, err := client.Do(req)
        //if err != nil {
        //    fmt.Println("Error sending request:", err)
        //    return
        //}
        //defer resp.Body.Close()

        // Check the response
        //if resp.StatusCode == http.StatusOK {
        //    fmt.Println("Message sent successfully!")
        //} else if resp.StatusCode == http.StatusNoContent {
        //    fmt.Println("Message send, but there was a response with no content (HTTP 204)")
        //} else {
        //    fmt.Println("Failed to send message. Status code:", resp.StatusCode)
        //}

	// fmt.Printf("Currently unused data: duration: %s json data: %s\n", duration, jsonObject)
}

func handleDel(ip string, duration string, reason string, jsonObject map[string]interface{}) {
	cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--del=%s", ip))
	out, err := cmd.Output()
	if err != nil {
		fmt.Println("Error deleting IP:", err)
		return
	}
	fmt.Println(string(out))

        //hostname, err := os.Hostname()
        //if err != nil {
        //    fmt.Println("Error:", err)
        //}

        //currentDate := time.Now().Format("2006-01-02 15:04")

        //webhookURL := "https://discord.com/api/webhooks/1215603532451946566/JsMb7nSHAHOIZ4FJZf8xu0I495dsh1_5LV5x7x52KQBfYM5BoamNCDPAiRbk7yv6q-cX"
        // embedColor := 0x1fff00

        // Create the embed
        // embed := map[string]interface{} {
        //    "Banned IP":         ip,
        //    "Comment":           reason,
        //    "Was banned for":    duration,
        //    "JSON_DATA":         jsonObject,
        //}

        //payload := map[string]interface{} {
            //"content": fmt.Sprintf("Unbanned IP: %s\nComment: %s\nWas banned for: %s\nJSON_DATA: %s", ip, reason, duration, jsonObject),
        //   "embeds": []map[string]interface{}{embed},
        //}
        //payload := map[string]string {
        //    "content": fmt.Sprintf("Hostname: %s\nCurrent date: %s\nUnbanned IP: %s\nComment: %s\nJSON_DATA: %s", hostname, currentDate, ip, reason, jsonObject),
        //}
        //jsonData, err := json.Marshal(payload)
        //if err != nil {
        //    fmt.Println("Error marshaling JSON:", err)
        //    return
        //}
        // Create the HTTP request
        //req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
        //if err != nil {
        //    fmt.Println("Error creating request:", err)
        //    return
        //}
        //req.Header.Set("Content-Type", "application/json")

        // Send the HTTP request
        //client := &http.Client{}
        //resp, err := client.Do(req)
        //if err != nil {
        //    fmt.Println("Error sending request:", err)
        //    return
        //}
        //defer resp.Body.Close()

        // Check the response
        //if resp.StatusCode == http.StatusOK {
        //    fmt.Println("Message sent successfully!")
        //} else if resp.StatusCode == http.StatusNoContent {
        //    fmt.Println("Message send, but there was a response with no content (HTTP 204)")
        //} else {
        //    fmt.Println("Failed to send message. Status code:", resp.StatusCode)
        //}
	// fmt.Printf("Currently unused data: reason: %s duration: %s json data: %s\n", reason, duration, jsonObject)
}

func processCommand(command string, ip string, duration string, reason string, jsonObject map[string]interface{}) {
	switch command {
	case "add":
		handleAdd(ip, duration, reason, jsonObject)
	case "del":
		handleDel(ip, duration, reason, jsonObject)
	default:
		fmt.Println("Invalid command")
	}
}

func main() {
	if len(os.Args) != 6 {
		fmt.Println("Usage: go run main.go <command> <ip> <duration> <reason> <json_object>")
		os.Exit(1)
	}

	command := os.Args[1]
	ip := os.Args[2]
	duration := os.Args[3]
	reason := os.Args[4]
	jsonStr := os.Args[5]

	var jsonObject map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &jsonObject)
	if err != nil {
		fmt.Println("Invalid JSON object:", err)
		os.Exit(1)
	}

	processCommand(command, ip, duration, reason, jsonObject)
}
