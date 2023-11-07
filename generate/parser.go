package generate

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"github.com/zeromicro/go-zero/tools/goctl/api/spec"
	"github.com/zeromicro/go-zero/tools/goctl/plugin"
)

var strColon = []byte(":")

const (
	defaultOption   = "default"
	stringOption    = "string"
	optionalOption  = "optional"
	omitemptyOption = "omitempty"
	optionsOption   = "options"
	rangeOption     = "range"
	exampleOption   = "example"
	optionSeparator = "|"
	equalToken      = "="
)

var excludePaths = []string{"/swagger", "/swagger-json"}
var excludeTagKeys = map[string]string{"header": "", "path": "", "form": ""}

func parseRangeOption(option string) (float64, float64, bool) {
	const str = "\\[([+-]?\\d+(\\.\\d+)?):([+-]?\\d+(\\.\\d+)?)\\]"
	result := regexp.MustCompile(str).FindStringSubmatch(option)
	if len(result) != 5 {
		return 0, 0, false
	}

	min, err := strconv.ParseFloat(result[1], 64)
	if err != nil {
		return 0, 0, false
	}

	max, err := strconv.ParseFloat(result[3], 64)
	if err != nil {
		return 0, 0, false
	}

	if max < min {
		return min, min, true
	}
	return min, max, true
}

func applyGenerate(p *plugin.Plugin, host string, basePath string) (*swaggerObject, error) {
	title, _ := strconv.Unquote(p.Api.Info.Properties["title"])
	version, _ := strconv.Unquote(p.Api.Info.Properties["version"])
	desc, _ := strconv.Unquote(p.Api.Info.Properties["desc"])

	s := swaggerObject{
		Swagger:           "2.0",
		Schemes:           []string{"http", "https"},
		Consumes:          []string{"application/json"},
		Produces:          []string{"application/json"},
		Paths:             make(swaggerPathsObject),
		Definitions:       make(swaggerDefinitionsObject),
		StreamDefinitions: make(swaggerDefinitionsObject),
		Info: swaggerInfoObject{
			Title:       title,
			Version:     version,
			Description: desc,
		},
	}
	if len(host) > 0 {
		s.Host = host
	}
	if len(basePath) > 0 {
		s.BasePath = basePath
	}

	s.SecurityDefinitions = swaggerSecurityDefinitionsObject{}
	newSecDefValue := swaggerSecuritySchemeObject{}
	newSecDefValue.Name = "Authorization"
	newSecDefValue.Description = "Enter JWT Bearer token **_only_**"
	newSecDefValue.Type = "apiKey"
	newSecDefValue.In = "header"
	s.SecurityDefinitions["apiKey"] = newSecDefValue
	s.Security = append(s.Security, swaggerSecurityRequirementObject{"apiKey": []string{}})

	requestResponseRefs := refMap{}
	renderServiceRoutes(p.Api.Service, p.Api.Service.Groups, s.Paths, requestResponseRefs)
	m := messageMap{}

	renderReplyAsDefinition(s.Definitions, m, p.Api.Types, requestResponseRefs)

	return &s, nil
}

func renderServiceRoutes(service spec.Service, groups []spec.Group, paths swaggerPathsObject, requestResponseRefs refMap) {
	//log.Printf("[service]:%+v", service)

	for _, group := range groups {
		log.Printf("[group]:%+v", group)
		for _, route := range group.Routes {
			// route:{AtServerAnnotation:{Properties:map[]} Method:get Path:/ RequestType:<nil> ResponseType:{RawName:IndexResponse Members:[{Name:Msg Type:{RawName:string} Tag:`json:"msg"` Comment: Docs:[] IsInline:false}] Docs:[]} Docs:[] Handler:IndexHandler AtDoc:{Properties:map[] Text:"首页"} HandlerDoc:[] HandlerComment:[] Doc:[] Comment:[]}
			//log.Printf("[route]:%+v", route)

			path := group.GetAnnotation("prefix") + route.Path
			if path[0] != '/' {
				path = "/" + path
			}

			isExclude := false
			for _, excludePath := range excludePaths {
				if path == excludePath {
					isExclude = true
					break
				}
			}
			if isExclude {
				continue
			}
			parameters := swaggerParametersObject{}
			// 处理路径参数url tag:{path}
			if countParams(path) > 0 {
				p := strings.Split(path, "/")
				for i := range p {
					part := p[i]
					// path 是靠分析url的/:确定的
					if strings.Contains(part, ":") {
						key := strings.TrimPrefix(p[i], ":")
						//path 有:cars变成{cars}
						path = strings.Replace(path, fmt.Sprintf(":%s", key), fmt.Sprintf("{%s}", key), 1)

						spo := swaggerParameterObject{
							Name:     key,
							In:       "path",
							Required: true,
							Type:     "string",
						}

						// extend the comment functionality
						// to allow query string parameters definitions
						// EXAMPLE:
						// @doc(
						// 	summary: "Get Cart"
						// 	description: "returns a shopping cart if one exists"
						// 	customerId: "customer id"
						// )
						//
						// the format for a parameter is
						// paramName: "the param description"
						//

						prop := route.AtDoc.Properties[key]
						if prop != "" {
							// remove quotes
							spo.Description = strings.Trim(prop, "\"")
						}

						parameters = append(parameters, spo)
					}
				}
			}
			if defineStruct, ok := route.RequestType.(spec.DefineStruct); ok {

				//处理header
				/*				for _, member := range defineStruct.Members {
								// {Name:Address Type:{RawName:string} Tag:`json:"address"` Comment: Docs:[] IsInline:false}
								// {Name:Auth Type:{RawName:string} Tag:`header:"auth,optional"` Comment://报文头 Docs:[] IsInline:false}
								//{Name:Where Type:{RawName:string} Tag:`form:"where"` Comment: Docs:[] IsInline:false}
								//{Name:Who Type:{RawName:string} Tag:`path:"who"` Comment: Docs:[] IsInline:false}
								//log.Printf("member:%+v", member)

								//处理匿名字段,api里面一般也没有匿名字段
								if member.Name == "" {
									memberDefineStruct, _ := member.Type.(spec.DefineStruct)
									for _, m := range memberDefineStruct.Members {
										if strings.Contains(m.Tag, "header") {
											parameters = append(parameters, renderStruct(m))
										}
									}
									continue
								}
							}*/

				for _, member := range defineStruct.Members {
					if strings.Contains(member.Tag, "path") {
						continue
					}
					if strings.Contains(member.Tag, "header") || strings.Contains(member.Tag, "form") {

						if embedStruct, isEmbed := member.Type.(spec.DefineStruct); isEmbed {
							for _, m := range embedStruct.Members {
								parameters = append(parameters, renderStruct(m))
							}
							continue
						}
						parameters = append(parameters, renderStruct(member))
					}
				}

				//处理非get请求
				if strings.ToUpper(route.Method) != http.MethodGet {

					//post轻轻也可能出现head

					//UploadRequest
					//Auth Where
					//body := route.RequestType.Name() //UploadRequest
					//content := fmt.Sprintf("%s\n%s\n", body, strings.Join(deleted, " "))
					//ioutil.WriteFile("/home/zh/body.txt", []byte(content), 0666)
					//if len(deleted) > 0 {
					//body = strings.Replace(body, fmt.Sprintf(`"%s"`, deleted[0]), "A", -1)

					//}

					reqRef := fmt.Sprintf("#/definitions/%s", route.RequestType.Name())

					if len(route.RequestType.Name()) > 0 {
						schema := swaggerSchemaObject{
							schemaCore: schemaCore{
								Ref: reqRef,
							},
						}

						parameter := swaggerParameterObject{
							Name:     "body",
							In:       "body",
							Required: true,
							Schema:   &schema,
						}
						doc := strings.Join(route.RequestType.Documents(), ",")
						doc = strings.Replace(doc, "//", "", -1)

						if doc != "" {
							parameter.Description = doc
						}

						parameters = append(parameters, parameter)
					}
				} //post
			}

			pathItemObject, ok := paths[path]
			if !ok {
				pathItemObject = swaggerPathItemObject{}
			}

			desc := "A successful response."
			respRef := ""
			if route.ResponseType != nil && len(route.ResponseType.Name()) > 0 {
				respRef = fmt.Sprintf("#/definitions/%s", route.ResponseType.Name())
			}
			tags := service.Name //默认取service的名字
			if value := group.GetAnnotation("group"); len(value) > 0 {
				tags = value //group 的名字
			}
			if value := group.GetAnnotation("swtags"); len(value) > 0 {
				tags = value
			}
			operationObject := &swaggerOperationObject{
				Tags:       []string{tags},
				Parameters: parameters,
				Responses: swaggerResponsesObject{
					"200": swaggerResponseObject{
						Description: desc,
						Schema: swaggerSchemaObject{
							schemaCore: schemaCore{
								Ref: respRef,
							},
						},
					},
				},
			}

			// set OperationID
			operationObject.OperationID = route.Handler

			for _, param := range operationObject.Parameters {
				if param.Schema != nil && param.Schema.Ref != "" {
					requestResponseRefs[param.Schema.Ref] = struct{}{}
				}
			}
			operationObject.Summary = strings.ReplaceAll(route.JoinedDoc(), "\"", "")

			if len(route.AtDoc.Properties) > 0 {
				operationObject.Description, _ = strconv.Unquote(route.AtDoc.Properties["description"])
			}

			operationObject.Description = strings.ReplaceAll(operationObject.Description, "\"", "")

			if group.Annotation.Properties["jwt"] != "" {
				operationObject.Security = &[]swaggerSecurityRequirementObject{{"apiKey": []string{}}}
			}

			switch strings.ToUpper(route.Method) {
			case http.MethodGet:
				pathItemObject.Get = operationObject
			case http.MethodPost:
				pathItemObject.Post = operationObject
			case http.MethodDelete:
				pathItemObject.Delete = operationObject
			case http.MethodPut:
				pathItemObject.Put = operationObject
			case http.MethodPatch:
				pathItemObject.Patch = operationObject
			}

			paths[path] = pathItemObject
		}
	}
}

func renderStruct(member spec.Member) swaggerParameterObject {
	tempKind := swaggerMapTypes[strings.Replace(member.Type.Name(), "[]", "", -1)]

	ftype, format, ok := primitiveSchema(tempKind, member.Type.Name())
	if !ok {
		ftype = tempKind.String()
		format = "UNKNOWN"
	}

	sp := swaggerParameterObject{In: "query", Type: ftype, Format: format}
	sp.Schema = new(swaggerSchemaObject)

	for i, tag := range member.Tags() {
		sp.Name = tag.Name //字段名字.
		if i == 0 {
			sp.In = tag.Key //gozero的tag一定要在最前面
		}

		if len(tag.Options) == 0 {
			sp.Required = true
			continue
		}

		required := true
		for _, option := range tag.Options {
			if strings.HasPrefix(option, optionsOption) {
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					sp.Enum = strings.Split(segs[1], optionSeparator)
				}
			}

			if strings.HasPrefix(option, rangeOption) {
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					min, max, ok := parseRangeOption(segs[1])
					if ok {
						sp.Schema.Minimum = min
						sp.Schema.Maximum = max
					}
				}
			}

			if strings.HasPrefix(option, defaultOption) {
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					sp.Default = segs[1]
				}
			} else if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
				required = false
			}

			if strings.HasPrefix(option, exampleOption) {
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					sp.Example = segs[1]
				}
			}
		}
		sp.Required = required
	}

	if len(member.Comment) > 0 {
		sp.Description = strings.TrimLeft(member.Comment, "//")
	}

	return sp
}

func renderReplyAsDefinition(d swaggerDefinitionsObject, m messageMap, p []spec.Type, refs refMap) {
	for _, i2 := range p {
		schema := swaggerSchemaObject{
			schemaCore: schemaCore{
				Type: "object",
			},
		}
		defineStruct, _ := i2.(spec.DefineStruct)

		schema.Title = defineStruct.Name() //结构体的名字

		//{Name:Who Type:{RawName:string} Tag:`path:"who"` Comment: Docs:[] IsInline:false}

		for _, member := range defineStruct.Members {

			//header path form 不在作为json字段显示
			if hasExcluParameters(member) {
				continue
			}
			kv := keyVal{Value: schemaOfField(member)}
			kv.Key = member.Name
			if tag, err := member.GetPropertyName(); err == nil {
				kv.Key = tag
			}
			if kv.Key == "" {
				memberStruct, _ := member.Type.(spec.DefineStruct)
				for _, m := range memberStruct.Members {
					if strings.Contains(m.Tag, "header") {
						continue
					}

					mkv := keyVal{
						Value: schemaOfField(m),
						Key:   m.Name,
					}

					if tag, err := m.GetPropertyName(); err == nil {
						mkv.Key = tag
					}
					if schema.Properties == nil {
						schema.Properties = &swaggerSchemaObjectProperties{}
					}
					*schema.Properties = append(*schema.Properties, mkv)
				}
				continue
			}
			if schema.Properties == nil {
				schema.Properties = &swaggerSchemaObjectProperties{}
			}
			*schema.Properties = append(*schema.Properties, kv)

			for _, tag := range member.Tags() {
				if len(tag.Options) == 0 {
					if !contains(schema.Required, tag.Name) && tag.Name != "required" {
						schema.Required = append(schema.Required, tag.Name)
					}
					continue
				}

				required := true
				for _, option := range tag.Options {
					// case strings.HasPrefix(option, defaultOption):
					// case strings.HasPrefix(option, optionsOption):

					if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
						required = false
					}
				}

				if required && !contains(schema.Required, tag.Name) {
					schema.Required = append(schema.Required, tag.Name)
				}
			}
		}

		d[i2.Name()] = schema
	}
}

func hasExcluParameters(member spec.Member) bool {
	for _, tag := range member.Tags() {
		if _, ok := excludeTagKeys[tag.Key]; ok {
			return true
		}
	}

	return false
}
func hasPathParameters(member spec.Member) bool {
	for _, tag := range member.Tags() {
		if tag.Key == "path" {
			return true
		}
	}

	return false
}

func hasHeaderParameters(member spec.Member) bool {
	for _, tag := range member.Tags() {
		if tag.Key == "header" {
			return true
		}
	}

	return false
}

func schemaOfField(member spec.Member) swaggerSchemaObject {
	////{Name:Who Type:{RawName:string} Tag:`path:"who"` Comment: Docs:[] IsInline:false}
	ret := swaggerSchemaObject{}

	var core schemaCore
	// spew.Dump(member)
	kind := swaggerMapTypes[member.Type.Name()]
	var props *swaggerSchemaObjectProperties

	comment := member.GetComment()
	comment = strings.Replace(comment, "//", "", -1)

	switch ft := kind; ft {
	case reflect.Invalid: //[]Struct 也有可能是 Struct
		// []Struct
		// map[ArrayType:map[Star:map[StringExpr:UserSearchReq] StringExpr:*UserSearchReq] StringExpr:[]*UserSearchReq]
		refTypeName := strings.Replace(member.Type.Name(), "[", "", 1)
		refTypeName = strings.Replace(refTypeName, "]", "", 1)
		refTypeName = strings.Replace(refTypeName, "*", "", 1)
		refTypeName = strings.Replace(refTypeName, "{", "", 1)
		refTypeName = strings.Replace(refTypeName, "}", "", 1)
		// interface

		if refTypeName == "interface" {
			core = schemaCore{Type: "object"}
		} else if refTypeName == "mapstringstring" {
			core = schemaCore{Type: "object"}
		} else if strings.HasPrefix(refTypeName, "[]") {
			core = schemaCore{Type: "array"}

			tempKind := swaggerMapTypes[strings.Replace(refTypeName, "[]", "", -1)]
			ftype, format, ok := primitiveSchema(tempKind, refTypeName)
			if ok {
				core.Items = &swaggerItemsObject{Type: ftype, Format: format}
			} else {
				core.Items = &swaggerItemsObject{Type: ft.String(), Format: "UNKNOWN"}
			}

		} else {
			core = schemaCore{
				Ref: "#/definitions/" + refTypeName,
			}
		}
	case reflect.Slice:
		tempKind := swaggerMapTypes[strings.Replace(member.Type.Name(), "[]", "", -1)]
		ftype, format, ok := primitiveSchema(tempKind, member.Type.Name())

		if ok {
			core = schemaCore{Type: ftype, Format: format}
		} else {
			core = schemaCore{Type: ft.String(), Format: "UNKNOWN"}
		}
	default:
		ftype, format, ok := primitiveSchema(ft, member.Type.Name())
		if ok {
			core = schemaCore{Type: ftype, Format: format}
		} else {
			core = schemaCore{Type: ft.String(), Format: "UNKNOWN"}
		}
	}

	switch ft := kind; ft {
	case reflect.Slice:
		ret = swaggerSchemaObject{
			schemaCore: schemaCore{
				Type:  "array",
				Items: (*swaggerItemsObject)(&core),
			},
		}
	case reflect.Invalid:
		// 判断是否数组
		if strings.HasPrefix(member.Type.Name(), "[]") {
			ret = swaggerSchemaObject{
				schemaCore: schemaCore{
					Type:  "array",
					Items: (*swaggerItemsObject)(&core),
				},
			}
		} else {
			ret = swaggerSchemaObject{
				schemaCore: core,
				Properties: props,
			}
		}
		if strings.HasPrefix(member.Type.Name(), "map") {
			fmt.Println("暂不支持map类型")
		}
	default:
		ret = swaggerSchemaObject{
			schemaCore: core,
			Properties: props,
		}
	}
	ret.Description = comment

	for _, tag := range member.Tags() {
		if len(tag.Options) == 0 {
			continue
		}
		for _, option := range tag.Options {
			switch {
			case strings.HasPrefix(option, defaultOption):
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					ret.Default = segs[1]
				}
			case strings.HasPrefix(option, optionsOption):
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					ret.Enum = strings.Split(segs[1], optionSeparator)
				}
			case strings.HasPrefix(option, rangeOption):
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					min, max, ok := parseRangeOption(segs[1])
					if ok {
						ret.Minimum = min
						ret.Maximum = max
					}
				}
			case strings.HasPrefix(option, exampleOption):
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					ret.Example = segs[1]
				}
			}
		}
	}

	return ret
}

// https://swagger.io/specification/ Data Types
func primitiveSchema(kind reflect.Kind, t string) (ftype, format string, ok bool) {
	switch kind {
	case reflect.Int:
		return "integer", "int32", true
	case reflect.Uint:
		return "integer", "uint32", true
	case reflect.Int8:
		return "integer", "int8", true
	case reflect.Uint8:
		return "integer", "uint8", true
	case reflect.Int16:
		return "integer", "int16", true
	case reflect.Uint16:
		return "integer", "uin16", true
	case reflect.Int64:
		return "integer", "int64", true
	case reflect.Uint64:
		return "integer", "uint64", true
	case reflect.Bool:
		return "boolean", "boolean", true
	case reflect.String:
		return "string", "", true
	case reflect.Float32:
		return "number", "float", true
	case reflect.Float64:
		return "number", "double", true
	case reflect.Slice:
		return strings.Replace(t, "[]", "", -1), "", true
	default:
		return "", "", false
	}
}

// StringToBytes converts string to byte slice without a memory allocation.
func stringToBytes(s string) (b []byte) {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}

func countParams(path string) uint16 {
	var n uint16
	s := stringToBytes(path)
	n += uint16(bytes.Count(s, strColon))
	return n
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func del(s []string, str string) []string {
	index := -1
	for i, v := range s {
		if v == str {
			index = i
			break
		}
	}
	if index != -1 {
		s = append(s[:index], s[index+1:]...) // 删除中间1个元素
	}
	return s
}

func parserTags(option string, ret *swaggerSchemaObject) {
	option = strings.Trim(option, " ")
	switch {
	case strings.HasPrefix(option, defaultOption):
		segs := strings.Split(option, equalToken)
		if len(segs) == 2 {
			ret.Default = segs[1]
		}
	case strings.HasPrefix(option, optionsOption):
		segs := strings.Split(option, equalToken)
		if len(segs) == 2 {
			enumArray := segs[1]
			var enums []string
			if strings.Contains(enumArray, optionSeparator) {
				enums = strings.Split(option, optionSeparator)
				enums[0] = enums[0][8:]
			} else {
				enums = []string{enumArray}
			}
			ret.Enum = enums
		}
	case strings.HasPrefix(option, rangeOption):
		segs := strings.Split(option, equalToken)
		if len(segs) != 2 {
			return
		}
		rangeStr := segs[1]
		if !strings.Contains(rangeStr, ":") {
			return
		}
		rangeArray := strings.Split(rangeStr, ":")
		if len(rangeArray) != 2 {
			return
		}
		num1, num2 := rangeArray[0][1:], rangeArray[1][:len(rangeArray[1])-1]
		float1, err := strconv.ParseFloat(num1, 64)
		if err == nil {
			ret.Minimum = float1
		}
		float2, err := strconv.ParseFloat(num2, 64)
		if err == nil {
			ret.Maximum = float2
		}
	}
}
