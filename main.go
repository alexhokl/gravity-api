package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/urfave/cli"
)

// Configuration stores configuration information
type Configuration struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// Command indicates the requirements of executing a command
type Command interface {
	GetCommand(name string, isVerbose bool, args []string) *exec.Cmd
}

// CommandLine executes commands
type CommandLine struct {
}

// GetCommand retrieve the command to be executed
func (c *CommandLine) GetCommand(name string, isVerbose bool, args []string) *exec.Cmd {
	if isVerbose {
		fmt.Println("Command executed:", name, args)
	}
	return exec.Command(name, args...)
}

const configuationFilename = ".gravity-api.yaml"
const responseTempFilename = "gravity-api-response"

func main() {
	resourceFlag := cli.StringFlag{
		Name:  "resource, r",
		Usage: "URI to the resource API",
	}
	selectorFlag := cli.StringFlag{
		Name:  "selector, s",
		Usage: "jq selector to the json response",
		Value: ".",
	}
	paramFileFlag := cli.StringFlag{
		Name:  "file, f",
		Usage: "JSON data to be included in query string",
	}
	fileFlag := cli.StringFlag{
		Name:  "file, f",
		Usage: "JSON data (this cannot be used with parameter --data)",
	}
	dataFlag := cli.StringFlag{
		Name:  "data, d",
		Usage: "JSON data (this cannot be used with parameter --file)",
	}
	noResponseFlag := cli.BoolFlag{
		Name:  "no-response",
		Usage: "Do not show HTTP response",
	}
	noStatFlag := cli.BoolFlag{
		Name:  "no-stat",
		Usage: "Do not show statistics",
	}

	queryFlags := []cli.Flag{resourceFlag, selectorFlag, paramFileFlag, noResponseFlag, noStatFlag}
	dataFlags := []cli.Flag{resourceFlag, selectorFlag, fileFlag, dataFlag, noResponseFlag, noStatFlag}

	app := cli.NewApp()
	app.Usage = "A CLI tool to interact with Gravity APIs"
	app.Commands = []cli.Command{
		{
			Name:  "configure",
			Usage: "Configure API's URL",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "url, u",
					Usage: "URL to the API",
				},
				cli.StringFlag{
					Name:  "token, t",
					Usage: "Optional, token to make API calls",
				},
			},
			Action: configureCommand,
			Subcommands: []cli.Command{
				cli.Command{
					Name:   "show",
					Usage:  "Show the current configuration",
					Action: showConfigurationCommand,
				},
			},
		},
		{
			Name:  "login",
			Usage: "Log onto the API and store the access token",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "username, u",
					Usage: "Username of API",
				},
				cli.StringFlag{
					Name:  "password, p",
					Usage: "Password of login of API",
				},
			},
			Action: loginCommand,
		},
		{
			Name:   "get",
			Usage:  "Retrieve data from API",
			Flags:  queryFlags,
			Action: getCommand,
		},
		{
			Name:   "post",
			Usage:  "Create resource via API",
			Flags:  dataFlags,
			Action: postCommand,
		},
		{
			Name:   "put",
			Usage:  "Modified data via API",
			Flags:  dataFlags,
			Action: putCommand,
		},
		{
			Name:   "patch",
			Usage:  "Patch data via API",
			Flags:  dataFlags,
			Action: patchCommand,
		},
		{
			Name:   "delete",
			Usage:  "Delete data from API",
			Flags:  queryFlags,
			Action: deleteCommand,
		},
	}
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "Verbose mode",
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func configureCommand(c *cli.Context) error {
	if c.String("url") == "" && c.String("token") == "" {
		return errors.New("No parameters are specified. See --help for options")
	}

	config := Configuration{}

	isCreate := true
	configPath, errHome := getConfigPath(configuationFilename)
	if errHome != nil {
		return errHome
	}
	if _, errExist := os.Stat(configPath); errExist == nil {
		if err := getStoredConfiguration(configPath, &config); err != nil {
			return err
		}
		isCreate = false
	}
	bytes, errSerialise := getConfigurationBytes(c.String("url"), c.String("token"), &config)
	if errSerialise != nil {
		return errSerialise
	}
	writeConfiguration(configPath, bytes)

	if isCreate {
		fmt.Printf("Configuration file %s has been created.\n", configPath)
	} else {
		fmt.Printf("Configuration file %s has been updated.\n", configPath)
	}
	return nil
}

func showConfigurationCommand(c *cli.Context) error {
	configPath, errHome := getConfigPath(configuationFilename)
	if errHome != nil {
		return errHome
	}
	config := Configuration{}
	if err := getStoredConfiguration(configPath, &config); err != nil {
		return err
	}
	fmt.Printf("url: %s\n", config.URL)
	fmt.Printf("token: %s\n", config.Token)
	return nil
}

func loginCommand(c *cli.Context) error {
	if c.String("username") == "" {
		return errors.New("Parameter --username must be specified")
	}
	if c.String("password") == "" {
		return errors.New("Parameter --password must be specified")
	}
	configPath, errHome := getConfigPath(configuationFilename)
	if errHome != nil {
		return errHome
	}
	config := Configuration{}
	if err := getStoredConfiguration(configPath, &config); err != nil {
		return err
	}
	cmd := CommandLine{}
	output, err := executeHTTPCommand(&cmd, c.GlobalBool("verbose"), "", []string{
		"-X",
		"POST",
		"-d",
		fmt.Sprintf("grant_type=password&username=%s&password=%s", c.String("username"), c.String("password")),
		fmt.Sprintf("%s/login", config.URL),
	})
	if err != nil {
		return err
	}
	fmt.Println(output)
	if !strings.Contains(output, "200 OK") {
		return errors.New("Unable to login")
	}
	jqOutput, errJq := executeJqCommand(&cmd, c.GlobalBool("verbose"), []string{
		".access_token",
	})
	if errJq != nil {
		return errJq
	}
	token := strings.Replace(strings.Replace(jqOutput, "\"", "", -1), "\n", "", -1)
	bytes, errSerialise := getConfigurationBytes(config.URL, token, &config)
	if errSerialise != nil {
		return errSerialise
	}
	writeConfiguration(configPath, bytes)

	fmt.Printf("Configuration file %s has been updated.\n", configPath)

	return nil
}

func getCommand(c *cli.Context) error {
	cmd := CommandLine{}
	return executeQueryStringCommands(c, &cmd, "GET")
}

func postCommand(c *cli.Context) error {
	cmd := CommandLine{}
	return executeDataCommands(c, &cmd, "POST")
}

func putCommand(c *cli.Context) error {
	cmd := CommandLine{}
	return executeDataCommands(c, &cmd, "PUT")
}

func patchCommand(c *cli.Context) error {
	cmd := CommandLine{}
	return executeDataCommands(c, &cmd, "PATCH")
}

func deleteCommand(c *cli.Context) error {
	cmd := CommandLine{}
	return executeQueryStringCommands(c, &cmd, "DELETE")
}

func executeDataCommands(c *cli.Context, cmd Command, verb string) error {
	if c.String("data") != "" && c.String("file") != "" {
		return errors.New("Parameter --data cannot be used with parameter --file")
	}

	json, errData := getData(c)
	if errData != nil {
		return errData
	}

	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	return executeCommands(config, cmd, verb, c.String("resource"), "", json, c.String("selector"), !c.Bool("no-stat"), !c.Bool("no-response"), c.GlobalBool("verbose"))
}

func executeQueryStringCommands(c *cli.Context, cmd Command, verb string) error {
	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	queryString := ""
	if c.String("file") != "" {
		queryStr, errJSON := getQueryStringFromJSONFile(c.String("file"))
		if errJSON != nil {
			return errJSON
		}
		queryString = queryStr
	}

	return executeCommands(config, cmd, verb, c.String("resource"), queryString, "", c.String("selector"), !c.Bool("no-stat"), !c.Bool("no-response"), c.GlobalBool("verbose"))
}

func executeCommands(config *Configuration, cmd Command, verb string, resource string, queryString string, jsonData string, selector string, isShowStat bool, isShowResponse bool, isVerbose bool) error {
	args := []string{
		"-X",
		verb,
	}

	if jsonData != "" {
		args = append(args, "-H", "Content-Type: application/json", "-d", jsonData)
	}

	url := fmt.Sprintf("%s%s%s", config.URL, resource, queryString)
	args = append(args, url)

	fmt.Printf("[%s] %s\n", verb, url)
	output, err := executeHTTPCommand(cmd, isVerbose, config.Token, args)
	if err != nil {
		return err
	}
	if isShowStat {
		fmt.Println(output)
	}

	jqOutput, errJq := executeJqCommand(cmd, isVerbose, []string{selector})
	if errJq != nil {
		return errJq
	}
	if isShowResponse {
		fmt.Println(jqOutput)
	}

	return nil
}

func executeHTTPCommand(c Command, isVerbose bool, token string, args []string) (string, error) {
	allArgs := []string{
		"-o",
		filepath.Join(os.TempDir(), responseTempFilename),
	}
	if token != "" {
		allArgs = append(allArgs, "-H", fmt.Sprintf("authorization: Bearer %s", token))
	}
	for _, a := range args {
		allArgs = append(allArgs, a)
	}
	output, err := execute(c, "httpstat", isVerbose, allArgs)
	if err != nil {
		return "", err
	}
	return output, nil
}

func executeJqCommand(c Command, isVerbose bool, args []string) (string, error) {
	catCommand := c.GetCommand("cat", isVerbose, []string{filepath.Join(os.TempDir(), responseTempFilename)})
	jqCommand := c.GetCommand("jq", isVerbose, args)

	r, w := io.Pipe()
	catCommand.Stdout = w
	jqCommand.Stdin = r

	var jqBuffer bytes.Buffer
	jqCommand.Stdout = &jqBuffer

	catCommand.Start()
	jqCommand.Start()
	catCommand.Wait()
	w.Close()
	jqCommand.Wait()

	return string(jqBuffer.Bytes()), nil
}

func execute(c Command, name string, isVerbose bool, args []string) (string, error) {
	cmd := c.GetCommand(name, isVerbose, args)
	byteOutput, err := cmd.Output()
	return string(byteOutput), err
}

func writeConfiguration(configPath string, configurationBytes []byte) error {
	f, errOpen := os.Create(configPath)
	if errOpen != nil {
		return errOpen
	}
	defer f.Close()
	f.Write(configurationBytes)

	return nil
}

func getStoredConfiguration(configPath string, config *Configuration) error {
	bytes, errRead := ioutil.ReadFile(configPath)
	if errRead != nil {
		return errRead
	}
	errDeserialise := yaml.Unmarshal([]byte(bytes), config)
	if errDeserialise != nil {
		return errDeserialise
	}
	return nil
}

func getConfigurationBytes(url string, token string, config *Configuration) ([]byte, error) {
	if url != "" {
		config.URL = url
	}
	if token != "" {
		config.Token = token
	}
	bytes, errSerialise := yaml.Marshal(config)
	if errSerialise != nil {
		return nil, errSerialise
	}
	return bytes, nil
}

func getConfigPath(configuationFilename string) (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, configuationFilename), nil
}

func getValidatedConfiguration() (*Configuration, error) {
	configPath, errHome := getConfigPath(configuationFilename)
	if errHome != nil {
		return nil, errHome
	}
	config := Configuration{}
	if err := getStoredConfiguration(configPath, &config); err != nil {
		return nil, err
	}

	if config.URL == "" {
		return nil, errors.New("Please run command 'configure' and try again")
	}

	if config.Token == "" {
		return nil, errors.New("Please run command 'login' and try again")
	}
	return &config, nil
}

func isJSON(input string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(input), &js) == nil
}

func getJSONFromFile(path string) (string, error) {
	bytes, errRead := ioutil.ReadFile(path)
	if errRead != nil {
		return "", errRead
	}

	str := string(bytes)
	if !isJSON(str) {
		return "", errors.New("The specified file does not contain valid JSON")
	}
	line := strings.Replace(strings.Replace(str, "\r\n", "", -1), "\n", "", -1)
	return line, nil
}

func getData(c *cli.Context) (string, error) {
	if c.String("data") != "" {
		if !isJSON(c.String("data")) {
			return "", errors.New("Parameter --data is not in a proper JSON representation")
		}
		return c.String("data"), nil
	}

	if c.String("file") != "" {
		json, err := getJSONFromFile(c.String("file"))
		if err != nil {
			return "", err
		}
		return json, nil
	}
	return "", nil
}

func getQueryStringFromJSONFile(path string) (string, error) {
	bytes, errRead := ioutil.ReadFile(path)
	if errRead != nil {
		return "", errRead
	}

	str := string(bytes)
	if !isJSON(str) {
		return "", errors.New("The specified file does not contain valid JSON")
	}

	var objmap map[string]*json.RawMessage
	err := json.Unmarshal(bytes, &objmap)
	if err != nil {
		return "", err
	}

	array := []string{}
	for k, v := range objmap {
		if i, errInt := getIntFromJSON(v); errInt != nil {
			if f, errFloat := getFloatFromJSON(v); errFloat != nil {
				if f, errBool := getBoolFromJSON(v); errBool != nil {
					if s, errString := getStringFromJSON(v); errString != nil {
						// skip and do nothing
					} else {
						array = append(array, fmt.Sprintf("%s=%s", k, s))
					}
				} else {
					if f {
						array = append(array, fmt.Sprintf("%s=true", k))
					} else {
						array = append(array, fmt.Sprintf("%s=false", k))
					}
				}
			} else {
				array = append(array, fmt.Sprintf("%s=%f", k, f))
			}
		} else {
			array = append(array, fmt.Sprintf("%s=%d", k, i))
		}
	}

	builder := strings.Builder{}

	for i, v := range array {
		if i == 0 {
			builder.WriteString(fmt.Sprintf("?%s", v))
		} else {
			builder.WriteString(fmt.Sprintf("&%s", v))
		}
	}

	return builder.String(), nil
}

func getIntFromJSON(jsonObj *json.RawMessage) (int, error) {
	var value int
	err := json.Unmarshal(*jsonObj, &value)
	return value, err
}

func getFloatFromJSON(jsonObj *json.RawMessage) (float64, error) {
	var value float64
	err := json.Unmarshal(*jsonObj, &value)
	return value, err
}

func getBoolFromJSON(jsonObj *json.RawMessage) (bool, error) {
	var value bool
	err := json.Unmarshal(*jsonObj, &value)
	return value, err
}

func getStringFromJSON(jsonObj *json.RawMessage) (string, error) {
	var value string
	err := json.Unmarshal(*jsonObj, &value)
	return value, err
}
