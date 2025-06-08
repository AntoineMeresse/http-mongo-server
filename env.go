package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
)

type serverVar struct {
	dev      bool
	port     string
	mongoUri string
	mongoDb  string
}

func loadConfigPath(configPath string) map[string]any {
	file, err := os.Open(configPath)
	if err != nil {
		return map[string]any{}
	}
	defer file.Close()

	fileContent, err := io.ReadAll(file)
	if err != nil {
		return map[string]any{}
	}

	var res map[string]any
	err = json.Unmarshal(fileContent, &res)
	if err != nil {
		return map[string]any{}
	}

	return res
}

func getEnvVariables(configPath string) serverVar {
	vars := serverVar{}
	cfg := loadConfigPath(configPath)
	vars.dev = loadBoolVariable(cfg, "dev", false)
	vars.port = loadVariable(cfg, "serverPort", "8080")
	vars.mongoUri = loadVariable(cfg, "mongoUri", "mongodb://root:password@localhost:27017")
	vars.mongoDb = loadVariable(cfg, "mongoDb", "testDefault")
	return vars
}

func loadVariable(fileConfig map[string]any, envKey string, defaultValue string) string {
	v, exist := os.LookupEnv(envKey)
	if exist {
		return v
	}

	if v, ok := fileConfig[envKey]; ok {
		return fmt.Sprintf("%v", v)
	}

	return defaultValue
}

func loadBoolVariable(fileConfig map[string]any, envKey string, defaultValue bool) bool {
	boolValue, err := strconv.ParseBool(loadVariable(fileConfig, envKey, ""))
	if err != nil {
		return defaultValue
	}
	return boolValue
}
