package camelcase

import (
	"fmt"

	"github.com/iancoleman/strcase"
)

func ExampleSplit() {

	for _, c := range []string{
		"",
		"lowercase",
		"Class",
		"MyClass",
		"MyC",
		"HTML",
		"PDFLoader",
		"AString",
		"SimpleXMLParser",
		"vimRPCPlugin",
		"GL11Version",
		"99Bottles",
		"May5",
		"BFG9000",
		"BöseÜberraschung",
		"Two  spaces",
		"BadUTF8\xe2\xe2\xa1",
	} {
		fmt.Printf("%#v => %#v\n", c, Split(c))
	}

	// Output:
	// "" => []string{}
	// "lowercase" => []string{"lowercase"}
	// "Class" => []string{"Class"}
	// "MyClass" => []string{"My", "Class"}
	// "MyC" => []string{"My", "C"}
	// "HTML" => []string{"HTML"}
	// "PDFLoader" => []string{"PDF", "Loader"}
	// "AString" => []string{"A", "String"}
	// "SimpleXMLParser" => []string{"Simple", "XML", "Parser"}
	// "vimRPCPlugin" => []string{"vim", "RPC", "Plugin"}
	// "GL11Version" => []string{"GL", "11", "Version"}
	// "99Bottles" => []string{"99", "Bottles"}
	// "May5" => []string{"May", "5"}
	// "BFG9000" => []string{"BFG", "9000"}
	// "BöseÜberraschung" => []string{"Böse", "Überraschung"}
	// "Two  spaces" => []string{"Two", "  ", "spaces"}
	// "BadUTF8\xe2\xe2\xa1" => []string{"BadUTF8\xe2\xe2\xa1"}
}

func ExampleToCamelCase() {

	camelStr := ToCamelCase("HTTP_PORT")
	fmt.Println(camelStr)
	envVarStyle := strcase.ToScreamingSnake(camelStr)
	fmt.Println(envVarStyle) // 输出: HTTP_PORT

	// 另一个例子
	anotherStr := ToCamelCase("MY_APP_DATABASE_URL")
	fmt.Println(anotherStr)
	fmt.Println(strcase.ToScreamingSnake(anotherStr)) // 输出: MY_APP_DATABASE_URL
}
