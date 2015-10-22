package tinygo

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/kdada/tinygo/info"
)

//根据视图路径映射视图模板
var viewsMapper = map[string]*template.Template{}

// compileAllViews 根据tinyConfig.CompilePages设置编译全部视图
func compileAllViews() {
	if tinyConfig.precompile {
		filepath.Walk(tinyConfig.view, func(filePath string, fileInfo os.FileInfo, err error) error {
			if fileInfo != nil && !fileInfo.IsDir() && path.Ext(fileInfo.Name()) == info.DefaultTemplateExt {
				filePath = generateViewFilePath(filePath)
				if !isLayoutFile(filePath) {
					var tmpl, err = compileView(filePath)
					if err == nil {
						viewsMapper[filePath] = tmpl
					} else {
						fmt.Println(err)
					}
				}
			}
			return nil
		})
	}
}

// compileView 编译单个视图
// filePath: 相对于tinyConfig.ViewPath的文件路径,分隔符必须为/
func compileView(filePath string) (*template.Template, error) {
	var pathSlice = make([]string, 0, 2)
	var lastFile = filePath
	for lastFile != "" {
		pathSlice = append(pathSlice, getViewFilePath(lastFile))
		lastFile, _ = getLayoutFile(lastFile)
	}
	var tmpl, err = template.ParseFiles(pathSlice...)
	if err == nil {
		var name = filepath.Base(pathSlice[len(pathSlice)-1])
		tmpl = tmpl.Lookup(name)
	}
	return tmpl, err
}

// viewTemplate 返回指定视图的模板
// filePath:相对于tinyConfig.ViewPath的文件路径,分隔符必须为/
func viewTemplate(filePath string) *template.Template {
	var tmpl, ok = viewsMapper[filePath]
	if !ok {
		tmpl, err := compileView(filePath)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return tmpl
	}
	return tmpl
}

// partailViewTemplate 返回指定部分视图的模板
// filePath:相对于tinyConfig.ViewPath的文件路径,分隔符必须为/
func partialViewTemplate(filePath string) *template.Template {
	var tmpl, ok = viewsMapper[filePath]
	if !ok {
		tmpl, err := compileView(filePath)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return tmpl.Lookup(path.Base(filePath))
	}
	return tmpl.Lookup(path.Base(filePath))
}

// ParseTemplate 分析指定模板,如果模板不存在或者出错,则会返回HttpNotFound
// w:http响应写入器
// r:http请求
// path:相对于tinyConfig.ViewPath的文件路径,分隔符必须为/
// data:要解析到模板中的数据
func ParseTemplate(w http.ResponseWriter, r *http.Request, path string, data interface{}) {
	var tmpl = viewTemplate(path)
	if tmpl != nil {
		err := tmpl.Execute(w, data)
		if err != nil {
			fmt.Println(err)
			HttpNotFound(w, r)
		}
	}
}

// ParsePartialTemplate 分析指定部分模板,如果模板不存在或者出错,则会返回HttpNotFound
// 默认情况下,会首先寻找名为"Content"的模板并执行,如果"Content"模板不存在,则直接执行文件模板
// w:http响应写入器
// r:http请求
// path:相对于tinyConfig.ViewPath的文件路径,分隔符必须为/
// data:要解析到模板中的数据
func ParsePartialTemplate(w http.ResponseWriter, r *http.Request, path string, data interface{}) {
	var tmpl = partialViewTemplate(path)
	if tmpl != nil {
		content := tmpl.Lookup("Content")
		if content != nil {
			tmpl = content
		}
		err := tmpl.Execute(w, data)
		if err != nil {
			fmt.Println(err)
			HttpNotFound(w, r)
		}
	}
}

// mapStructToMap 将一个结构体所有字段(包括通过组合得来的字段)到一个map中
// value:结构体的反射值
// data:存储字段数据的map
func mapStructToMap(value reflect.Value, data map[interface{}]interface{}) {
	if value.Kind() == reflect.Struct {
		for i := 0; i < value.NumField(); i++ {
			var fieldValue = value.Field(i)
			if fieldValue.CanInterface() {
				var fieldType = value.Type().Field(i)
				if fieldType.Anonymous {
					//匿名组合字段,进行递归解析
					mapStructToMap(fieldValue, data)
				} else {
					//非匿名字段
					var fieldName = fieldType.Tag.Get("to")
					if fieldName == "" {
						fieldName = fieldType.Name
					}
					data[fieldName] = fieldValue.Interface()
				}
			}
		}
	}
}

// ParseUrlValueToStruct 将url值解析到结构体中
// urlValues:url值
// value:结构体的反射值
func ParseUrlValueToStruct(urlValues url.Values, value reflect.Value) {
	if value.Kind() == reflect.Struct {
		for i := 0; i < value.NumField(); i++ {
			var fieldValue = value.Field(i)
			var fieldType = value.Type().Field(i)
			if fieldType.Anonymous {
				//匿名组合字段,进行递归解析
				ParseUrlValueToStruct(urlValues, fieldValue)
			} else {
				//非匿名字段
				if fieldValue.CanSet() {
					var fieldName = fieldType.Tag.Get("from")
					if fieldName == "-" {
						//如果是-,则忽略当前字段
						continue
					}
					if fieldName == "" {
						//如果为空,则使用字段名
						fieldName = fieldType.Name
					}
					var urlValue = urlValues.Get(fieldName)
					switch fieldType.Type.Kind() {
					case reflect.Bool:
						result, err := strconv.ParseBool(urlValue)
						fieldValue.SetBool(result && err == nil)
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						result, err := strconv.ParseInt(urlValue, 10, 64)
						if err == nil {
							fieldValue.SetInt(result)
						} else {
							fieldValue.SetInt(0)
						}

					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
						result, err := strconv.ParseUint(urlValue, 10, 64)
						if err == nil {
							fieldValue.SetUint(result)
						} else {
							fieldValue.SetUint(0)
						}
					case reflect.Float32, reflect.Float64:
						result, err := strconv.ParseFloat(urlValue, 64)
						if err == nil {
							fieldValue.SetFloat(result)
						} else {
							fieldValue.SetFloat(0)
						}
					case reflect.Interface:
						fieldValue.Set(reflect.ValueOf(urlValue))
					case reflect.String:
						fieldValue.SetString(urlValue)
					case reflect.Struct:
						switch fieldType.Type.String() {
						case "time.Time":
							//仅支持该格式,忽略时区
							result, err := time.Parse("2006-01-02 15:04:05", urlValue)
							if err == nil {
								fieldValue.Set(reflect.ValueOf(result))
							}
						}
					case reflect.Slice:
						if fieldType.Type == reflect.TypeOf([]int(nil)) {
							stringValue := urlValues[fieldName]
							intValue := make([]int, len(stringValue), len(stringValue))
							for i := 0; i < len(intValue); i++ {
								intValue[i], _ = strconv.Atoi(stringValue[i])
							}
							fieldValue.Set(reflect.ValueOf(intValue))
						} else if fieldType.Type == reflect.TypeOf([]string(nil)) {
							stringValue := urlValues[fieldName]
							fieldValue.Set(reflect.ValueOf(stringValue))
						}
					}
				}

			}
		}
	}
}