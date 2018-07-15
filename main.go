package main

import (
	"bytes"
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
	Execute(name string, isVerbose bool, args []string) (string, error)
}

// CommandLine executes commands
type CommandLine struct {
}

// Execute executes the specified commands
func (c *CommandLine) Execute(name string, isVerbose bool, args []string) (string, error) {
	if isVerbose {
		fmt.Println("Command executed:", name, args)
	}
	byteOutput, err := exec.Command(name, args...).Output()
	return string(byteOutput), err

}

const configuationFilename = ".gravity-api.yaml"
const responseTempFilename = "gravity-api-response"

func main() {
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
			Name:  "get",
			Usage: "Retrieve data from API",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "resource, r",
					Usage: "URI to the resource API",
				},
				cli.StringFlag{
					Name:  "selector, s",
					Usage: "jq selector to the json response",
					Value: ".",
				},
			},
			Action: getCommand,
		},
		{
			Name:  "post",
			Usage: "Create resource via API",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "resource, r",
					Usage: "URI to the resource API",
				},
				cli.StringFlag{
					Name:  "data, d",
					Usage: "JSON data",
				},
				cli.StringFlag{
					Name:  "selector, s",
					Usage: "jq selector to the json response",
					Value: ".",
				},
			},
			Action: postCommand,
		},
		{
			Name:  "put",
			Usage: "Modified data via API",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "resource, r",
					Usage: "URI to the resource API",
				},
				cli.StringFlag{
					Name:  "data, d",
					Usage: "JSON data",
				},
				cli.StringFlag{
					Name:  "selector, s",
					Usage: "jq selector to the json response",
					Value: ".",
				},
			},
			Action: putCommand,
		},
		{
			Name:  "patch",
			Usage: "Patch data via API",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "resource, r",
					Usage: "URI to the resource API",
				},
				cli.StringFlag{
					Name:  "data, d",
					Usage: "JSON data",
				},
				cli.StringFlag{
					Name:  "selector, s",
					Usage: "jq selector to the json response",
					Value: ".",
				},
			},
			Action: patchCommand,
		},
		{
			Name:  "delete",
			Usage: "Delete data from API",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "resource, r",
					Usage: "URI to the resource API",
				},
				cli.StringFlag{
					Name:  "selector, s",
					Usage: "jq selector to the json response",
					Value: ".",
				},
			},
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
	output, err := executeHTTPCommand(&cmd, c.Bool("verbose"), "", []string{
		"-X",
		"POST",
		"-d",
		fmt.Sprintf("grant_type=password&username=%s&password=%s", c.String("username"), c.String("password")),
		fmt.Sprintf("%s/login", config.URL),
	})
	if err != nil {
		return err
	}
	if !strings.Contains(output, "200 OK") {
		return errors.New("Unable to login")
	}
	jqOutput, errJq := executeJqCommand(&cmd, c.Bool("verbose"), []string{
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
	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	cmd := CommandLine{}
	output, err := executeHTTPCommand(&cmd, c.Bool("verbose"), config.Token, []string{
		fmt.Sprintf("%s%s", config.URL, c.String("resource")),
	})
	if err != nil {
		return err
	}
	fmt.Println(output)

	jqOutput, errJq := executeJqCommand(&cmd, c.Bool("verbose"), []string{
		c.String("selector"),
	})
	if errJq != nil {
		return errJq
	}
	fmt.Println(jqOutput)

	return nil
}

func postCommand(c *cli.Context) error {
	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	cmd := CommandLine{}
	output, err := executeHTTPCommand(&cmd, c.Bool("verbose"), config.Token, []string{
		"-X",
		"POST",
		"-H",
		"Content-Type: application/json",
		"-d",
		c.String("data"),
		fmt.Sprintf("%s%s", config.URL, c.String("resource")),
	})
	if err != nil {
		return err
	}
	fmt.Println(output)

	jqOutput, errJq := executeJqCommand(&cmd, c.Bool("verbose"), []string{
		c.String("selector"),
	})
	if errJq != nil {
		return errJq
	}
	fmt.Println(jqOutput)

	return nil
}

func putCommand(c *cli.Context) error {
	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	cmd := CommandLine{}
	output, err := executeHTTPCommand(&cmd, c.Bool("verbose"), config.Token, []string{
		"-X",
		"PUT",
		"-H",
		"Content-Type: application/json",
		"-d",
		c.String("data"),
		fmt.Sprintf("%s%s", config.URL, c.String("resource")),
	})
	if err != nil {
		return err
	}
	fmt.Println(output)

	jqOutput, errJq := executeJqCommand(&cmd, c.Bool("verbose"), []string{
		c.String("selector"),
	})
	if errJq != nil {
		return errJq
	}
	fmt.Println(jqOutput)

	return nil
}

func patchCommand(c *cli.Context) error {
	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	cmd := CommandLine{}
	output, err := executeHTTPCommand(&cmd, c.Bool("verbose"), config.Token, []string{
		"-X",
		"PATCH",
		"-H",
		"Content-Type: application/json",
		"-d",
		c.String("data"),
		fmt.Sprintf("%s%s", config.URL, c.String("resource")),
	})
	if err != nil {
		return err
	}
	fmt.Println(output)

	jqOutput, errJq := executeJqCommand(&cmd, c.Bool("verbose"), []string{
		c.String("selector"),
	})
	if errJq != nil {
		return errJq
	}
	fmt.Println(jqOutput)

	return nil
}

func deleteCommand(c *cli.Context) error {
	config, errConfig := getValidatedConfiguration()
	if errConfig != nil {
		return errConfig
	}

	cmd := CommandLine{}
	output, err := executeHTTPCommand(&cmd, c.Bool("verbose"), config.Token, []string{
		"-X",
		"DELETE",
		fmt.Sprintf("%s%s", config.URL, c.String("resource")),
	})
	if err != nil {
		return err
	}
	fmt.Println(output)

	jqOutput, errJq := executeJqCommand(&cmd, c.Bool("verbose"), []string{
		c.String("selector"),
	})
	if errJq != nil {
		return errJq
	}
	fmt.Println(jqOutput)

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
	output, err := c.Execute("httpstat", isVerbose, allArgs)
	if err != nil {
		return "", err
	}
	return output, nil
}

func executeJqCommand(c Command, isVerbose bool, args []string) (string, error) {
	catCommand := exec.Command("cat", filepath.Join(os.TempDir(), responseTempFilename))
	jqCommand := exec.Command("jq", args...)

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
