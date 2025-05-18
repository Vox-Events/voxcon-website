package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type I18nID struct {
	Name    string `yaml:"name"`
	Code    string `yaml:"code"`
	Default bool   `yaml:"default" default:"false"`
}
type I18nFile struct {
	I18nID       `yaml:",inline"`
	Translations map[string]string `yaml:"translations"`
}
type TemplateData struct {
	Languages    []I18nID
	Currentlang  I18nID
	Translations map[string]string
	Extra        any
}

func main() {
	watchopt := len(os.Args) != 1 && os.Args[1] == "--watch"

	if watchopt {
		err := watch()
		if err != nil {
			panic(err)
		}
	} else {
		err := build()
		if err != nil {
			panic(err)
		}
		fmt.Println("done")
	}
}

func watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	walker := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			err = watcher.Add(path)
			if err != nil {
				return err
			}
		}
		return nil
	}
	err = filepath.WalkDir("../i18n", walker)
	if err != nil {
		return err
	}
	err = filepath.WalkDir("../src", walker)
	if err != nil {
		return err
	}
	for {
		time.Sleep(50 * time.Millisecond)
		flush(watcher.Events)
		err = build()
		if err != nil {
			fmt.Println("error building:", err)
		}
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("file watcher unexpectedly closed")
			}
			fmt.Println("event:", event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("file watcher unexpectedly closed")
			}
			return err
		}
	}
}

func flush(ch chan fsnotify.Event) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func build() error {
	i18nfiles, err := readI18nFiles()
	if err != nil {
		return err
	}
	i18nids := []I18nID{}
	defaulttranslations := map[string]string{}
	for _, f := range i18nfiles {
		i18nids = append(i18nids, f.I18nID)
	}

	b, err := os.ReadFile("../src/data.yaml")
	if err != nil {
		return err
	}
	extradata := make(map[string]interface{})
	yaml.Unmarshal(b, &extradata)

	for _, lang := range i18nfiles {
		templateData := TemplateData{i18nids, lang.I18nID, lang.Translations, extradata}
		for k, v := range defaulttranslations {
			if _, exist := templateData.Translations[k]; !exist {
				templateData.Translations[k] = v
			}
		}
		err = renderSite(templateData)
		if err != nil {
			return err
		}
	}

	cmd := exec.Command("rsync", "-a", "src/static/", "build")
	cmd.Dir = ".."
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("npx", "@tailwindcss/cli", "-i", "../src/css/main.css", "-o", "main.css")
	cmd.Dir = "../build"
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func readI18nFiles() ([]I18nFile, error) {
	i18nfiles := []I18nFile{}
	i18npaths, err := os.ReadDir("../i18n")
	if err != nil {
		return nil, err
	}
	for _, f := range i18npaths {
		if !f.IsDir() {
			b, err := os.ReadFile("../i18n/" + f.Name())
			if err != nil {
				return nil, err
			}
			i18n := I18nFile{}
			yaml.Unmarshal(b, &i18n)
			if i18n.Code == "" {
				code, _, _ := strings.Cut(f.Name(), ".")
				i18n.Code = code
			}
			i18nfiles = append(i18nfiles, i18n)
		}
	}
	return i18nfiles, nil
}

func renderSite(data TemplateData) error {
	builddir := "../build"
	if !data.Currentlang.Default {
		builddir += "/" + data.Currentlang.Code
	}
	err := os.MkdirAll(builddir, os.ModePerm)
	if err != nil {
		return err
	}

	tmpl := template.New("")
	tmpl.Funcs(template.FuncMap{
		"i18n": func(key string) string {
			return data.Translations[key]
		},
		"i18nhref": func(href string) string {
			if data.Currentlang.Default {
				return href
			} else {
				return "/" + data.Currentlang.Code + href
			}
		},
	})
	tmpl, err = tmpl.ParseGlob("../src/partials/*.html")
	if err != nil {
		return err
	}

	err = filepath.WalkDir("../src/content", func(path string, info fs.DirEntry, err error) error {
		if !info.IsDir() {
			bytes, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			tmpl2, err := tmpl.Clone()
			if err != nil {
				return err
			}
			tmpl2, err = tmpl2.New("page").Parse(string(bytes))
			if err != nil {
				return err
			}
			fmt.Printf("rendering [%s] %s\n", data.Currentlang.Code, path)
			target := strings.Replace(path, "../src/content", builddir, 1)
			lastslash := strings.LastIndex(target, "/")
			if lastslash != -1 {
				os.MkdirAll(target[0:lastslash], fs.ModePerm)
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			err = tmpl2.ExecuteTemplate(f, "index.html", data)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
