package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type serverVar struct {
	port     string
	mongoUri string
	mongoDb  string
}

func loadConfigPath(configPath string) map[string]any {
	file, err := os.Open(configPath)
	if err != nil {
		// fmt.Println("Can't open config file:", err)
		return map[string]any{}
	}
	defer file.Close()

	fileContent, err := io.ReadAll(file)
	if err != nil {
		// fmt.Println("Can't read config file:", err)
		return map[string]any{}
	}

	var res map[string]any
	err = json.Unmarshal(fileContent, &res)
	if err != nil {
		// fmt.Println("Can't unmarshal config file:", err)
		return map[string]any{}
	}

	return res
}

func getEnvVariables(configPath string) serverVar {
	vars := serverVar{}
	cfg := loadConfigPath(configPath)
	vars.port = loadVariable(cfg, "serverPort", "8080")
	vars.mongoUri = loadVariable(cfg, "mongoUri", "mongodb://root:password@localhost:27017")
	vars.mongoDb = loadVariable(cfg, "mongoDb", "testDefault")
	// fmt.Printf("Server var: %v\n", vars)
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
