package main

import (
	"fmt"

	"github.com/dyntrait/goctl-swagger/generate"
	"github.com/zeromicro/go-zero/tools/goctl/api/parser"
	plugin2 "github.com/zeromicro/go-zero/tools/goctl/plugin"
)

const userAPI = "./tests/packageserver.api"

func main() {
	result, err := parser.Parse(userAPI)
	if err != nil {
		fmt.Println(err)
		return
	}

	p := &plugin2.Plugin{
		Api:         result,
		ApiFilePath: userAPI,
		Style:       "",
		Dir:         ".",
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	err = generate.Do("./tests/user.json", "", "/", p)
	if err != nil {
		fmt.Println(err)
	}
}
