package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Docker image name constant
const DOCKER_IMAGE = "ubuntu:18.04"

// Define a struct to hold the JSON response from chatgpt API
type Response struct {
	RESULT string `json:"result"`
	ID     string `json:"id"`
}

// Define functions to construct response messages for different cases

// Construct a message to check whether the previous command succeeded
func check_success_request(result string) string {
	return "The result is " + result + "\nPlease tell me have I succeeded running your commands? Just return 'yes' or 'no'."
}

// Construct a message to request help with a failed command
func failed_command_request(command string, result string) string {
	return "I'm building my compile environment in a new " + DOCKER_IMAGE + " docker container. I am running command: \"" + command + "\"but it doesn't work.\nHere is the result:\n\"\n" + result + "\n\"\nJust return a shell script to help me fix it!"
}

// Construct a message to request the next command
func continue_request(result string) string {
	return "So what should I do next? Just return a shell script to help me!"
}

// Remove any "sudo" commands from a shell script
func remove_sudo(result string) string {
	return strings.ReplaceAll(result, "sudo ", "")
}

// Send a POST request to chatgpt API to request help
func post_request(request string, id string) (string, string) {

	// Create a new HTTP POST request with the JSON payload

	// Define the payload as a Go struct
	payload := struct {
		Zy string `json:"zy"`
		ID string `json:"id"`
	}{
		Zy: request,
		ID: id,
	}

	// Marshal the payload into JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", "https://zbvrsg.laf.dev/call_chatgpt", bytes.NewBuffer(payloadBytes))
	if err != nil {
		panic(err)
	}

	// Set the request headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request and get the response
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// Print the response status code and body
	fmt.Println("Response status:", resp.Status)

	// Decode the JSON response into a Response struct
	var responseObj Response
	err = json.NewDecoder(resp.Body).Decode(&responseObj)
	if err != nil {
		fmt.Println("Error decoding response:", err)
	}

	// Print the response fields
	fmt.Println("zy:", responseObj.RESULT)
	fmt.Println("id:", responseObj.ID)
	return responseObj.RESULT, responseObj.ID
}

// Execute a shell command and return the standard output, standard error, and exit code
func execute(command string) (string, string, int) {
	// Write the command to execute into exec.sh and execute it using bash.
	err := ioutil.WriteFile("exec.sh", []byte("#!/bin/bash\n"+command), 0644)

	// Create an exec.Cmd object to run the command.
	cmd := exec.Command("bash", "-c", "bash exec.sh")

	// Get the stdout and stderr pipes to retrieve the output of the command.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}

	// Start the command execution.
	err = cmd.Start()
	if err != nil {
		panic(err)
	}

	// Read the standard output and error output from the pipes.
	stdoutBytes, err := ioutil.ReadAll(stdout)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(stdoutBytes))
	stderrBytes, err := ioutil.ReadAll(stderr)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(stderrBytes))

	// Wait for the command to finish executing and get the exit code.
	err = cmd.Wait()
	if err != nil {
		// If the command exits with a non-zero exit code, return the standard output, error output, and exit code.
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			exitCode := exitErr.ExitCode()
			fmt.Println("Command exited with non-zero exit code:", exitCode)
			return string(stdoutBytes), string(stderrBytes), exitCode
		} else {
			panic(err)
		}
	} else {
		// If the command exits with a zero exit code, return the standard output, error output, and exit code.
		exitCode := cmd.ProcessState.ExitCode()
		fmt.Println("Command exited with zero exit code:", exitCode)
		return string(stdoutBytes), string(stderrBytes), exitCode
	}
}

func get_first_code_block(result string) (string, error) {
	// If the result string contains a code block delimited by triple backticks, return its content.
	if !strings.Contains(result, "```") {
		return strings.Split(strings.Split(result, "```")[1], "```")[0], nil
	}
	// If no code block is found, return an error.
	return "", fmt.Errorf("No code block found")
}

func main() {
	// Get the command to execute from command-line arguments.
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <command>")
		return
	}
	command := os.Args[1]

	// Initialize a stack to store the current commands.
	stack := []string{}

	// Add the initial command to the stack.
	stack = append(stack, command)

	// Initialize variables to track the current request ID and retry count.
	id := ""
	retry_count := 0

	// Loop until there are no more commands to execute or the maximum retry count is exceeded.
	for {
		if len(stack) == 0 {
			// If there are no more commands to execute, exit the loop.
			fmt.Println("No more commands to execute.")
			return
		}
		// Pop the most recent command from the stack.
		command = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fmt.Println("Executing command:", command)

		// Execute the command and get its output and exit code.
		stdout, stderr, exitCode := execute(command)
		// if the exit code is not zero, handle the failure
		if exitCode != 0 {
			// add the failed command back to the stack
			stack = append(stack, command)

			// create a failure result by combining stdout and stderr
			result := failed_command_request(command, stdout+stderr)

			// request advice from ChatGPT
			advice, nid := post_request(result, id)
			id = nid

			// extract the new command from the response and add it to the stack
			unprocessed_command, err := get_first_code_block(advice)
			if err != nil {
				// if no new command was found, remove "sudo" from the original command and add it back to the stack
				new_command := remove_sudo(unprocessed_command)
				fmt.Println("New command:", new_command)
				stack = append(stack, new_command)
				retry_count = 0
			} else {
				// if a new command was found, add it to the stack and retry the command
				retry_count++
				if retry_count > 3 {
					// if the maximum number of retries has been reached, exit the loop
					fmt.Println("Too many retries. Exit.")
					return
				}
				fmt.Println("No new command found. Retry.")
			}
		} else {
			// if the exit code is zero, the solution is solved and the loop can exit
			fmt.Println("Success! The solution is solved.")
			retry_count = 0
		}
	}
}
