package src

import (
	"encoding/json"
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

type ReverseProxy struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

type Config struct {
	Port string `yaml:"port"`
	Tls  struct {
		CrtFile string `yaml:"crt-file"`
		KeyFile string `yaml:"key-file"`
	} `yaml:"tls"`
	ReverseProxies []ReverseProxy `yaml:"reverse-proxies"`
	Redis          struct {
		Address string `yaml:"address"`
	} `yaml:"redis"`
}

func (config *Config) String() string {
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

func ParseConfig(configPath string) *Config {
	var yamlConfig Config
	{ // 解析yaml配置
		content, err := ioutil.ReadFile(configPath)
		if err != nil {
			panic(err)
		}
		err = yaml.Unmarshal(content, &yamlConfig)
		if err != nil {
			panic(err)
		}
		log.Println("读取配置文件: ")
		log.Println(&yamlConfig)
	}
	return &yamlConfig
}
