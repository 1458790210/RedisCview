package lib

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type systemConf struct {
	ConnectionTimeout int `json:"connectionTimeout"`
	ExecutionTimeout  int `json:"executionTimeout"`
	KeyScanLimits     int `json:"keyScanLimits"`
	RowScanLimits     int `json:"rowScanLimits"`
	DelRowLimits      int `json:"delRowLimits"`
}

type GlobalConfigStruct struct {
	Servers []redisServer `json:"servers"`
	System  systemConf    `json:"system"`
}

var globalConfigs GlobalConfigStruct

// _configFilePath 配置文件路径
var configPath = "./conf.json"
var defaultConfig = []byte(`{"servers":[{"id":1,"name":"localhost","host":"127.0.0.1","port":6379,"auth":"","dbNums":15}],"system":{"connectionTimeout":10,"executionTimeout":10,"keyScanLimits":100,"rowScanLimits":100,"delRowLimits":100}}`)

func init() {
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		err = ioutil.WriteFile(configPath, defaultConfig, 0755)
		checkErr(err)
	}

	conf, err := ioutil.ReadFile(configPath)
	checkErr(err)
	err = json.Unmarshal(conf, &globalConfigs)
	checkErr(err)
	maxServerID = 0

	for i := 0; i < len(globalConfigs.Servers); i++ {
		if maxServerID < globalConfigs.Servers[i].ID {
			maxServerID = globalConfigs.Servers[i].ID
		}
	}
}

func saveConf() error {
	data, err := json.Marshal(globalConfigs)
	logErr(err)
	if err == nil {
		err = ioutil.WriteFile(configPath, data, 0755)
	}
	return err
}
