package main

import (
    "os"
    "fmt"
)

func main() {
    f, err := os.Create("index.html")
    if err != nil {
        fmt.Println(err)
        return
    }
    defer f.Close()

    _, err = f.WriteString("<html><body>Hello World!</body></html>")
    if err != nil {
        fmt.Println(err)
        f.Close()
        return
    }
}
