package app

import "github.com/lioiioiioil/CampusJudge/internal/judge"

// Templates are complete stdin/stdout programs, not task-specific solutions.
// Automatic reflects the currently enabled executor, not just a source label.
type language struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Automatic bool   `json:"automatic"`
	Template  string `json:"template"`
}

var sourceLanguages = []language{
	{ID: "cpp", Name: "C++20", Template: `#include <iostream>
using namespace std;

int main() {
    ios::sync_with_stdio(false);
    cin.tie(nullptr);

    // Прочитайте входные данные и выведите ответ.

    return 0;
}
`},
	{ID: "python", Name: "Python 3", Template: `import sys


def main():
    # Прочитайте входные данные и выведите ответ.
    pass


if __name__ == "__main__":
    main()
`},
	{ID: "go", Name: "Go", Template: `package main

import (
    "bufio"
    "fmt"
    "os"
)

func main() {
    in := bufio.NewReader(os.Stdin)
    out := bufio.NewWriter(os.Stdout)
    defer out.Flush()

    // Прочитайте входные данные и выведите ответ.
    _ = in
    _ = fmt.Fprint
}
`},
	{ID: "java", Name: "Java", Template: `import java.io.*;
import java.util.*;

public class Main {
    public static void main(String[] args) throws Exception {
        Scanner in = new Scanner(System.in);
        // Прочитайте входные данные и выведите ответ.
    }
}
`},
	{ID: "javascript", Name: "JavaScript (Node.js)", Template: `const fs = require('fs');
const input = fs.readFileSync(0, 'utf8').trim();

// Прочитайте входные данные и выведите ответ.
`},
	{ID: "c", Name: "C17", Template: `#include <stdio.h>

int main(void) {
    // Прочитайте входные данные и выведите ответ.
    return 0;
}
`},
	{ID: "rust", Name: "Rust", Template: `use std::io::{self, Read};

fn main() {
    let mut input = String::new();
    io::stdin().read_to_string(&mut input).unwrap();
    // Прочитайте входные данные и выведите ответ.
}
`},
	{ID: "csharp", Name: "C#", Template: `using System;

class Program {
    static void Main() {
        // Прочитайте входные данные и выведите ответ.
    }
}
`},
	{ID: "kotlin", Name: "Kotlin", Template: `fun main() {
    // Прочитайте входные данные и выведите ответ.
}
`},
	{ID: "pascal", Name: "Pascal", Template: `program Solution;
begin
    { Прочитайте входные данные и выведите ответ. }
end.
`},
	{ID: "other", Name: "Другой язык", Template: ""},
}

func languageSupported(id string) bool {
	for _, candidate := range sourceLanguages {
		if candidate.ID == id {
			return true
		}
	}
	return false
}

func automaticLanguage(id string) bool {
	return judge.Supports(id)
}

func languageCatalog(automatic bool) []language {
	result := append([]language(nil), sourceLanguages...)
	for i := range result {
		result[i].Automatic = automatic && automaticLanguage(result[i].ID)
	}
	return result
}

func (a *App) judgingMode() string {
	if a.runner != nil {
		return "automatic"
	}
	return "manual"
}
