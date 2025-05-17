package main

import (
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type I18nID struct {
	Name    string `yaml:"name"`
	Path    string `yaml:"path"`
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
	err = watcher.Add("../i18n")
	if err != nil {
		return err
	}
	err = watcher.Add("../src")
	if err != nil {
		return err
	}
	for {
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

func build() error {
	i18nfiles, err := readI18nFiles()
	if err != nil {
		return err
	}
	i18nids := []I18nID{}
	defaultlang := I18nID{}
	defaulttranslations := map[string]string{}
	for _, f := range i18nfiles {
		i18nids = append(i18nids, f.I18nID)
		if f.Default {
			if defaultlang.Default {
				panic(fmt.Errorf("cannot have more than one default language, '%s' and '%s' both marked default", defaultlang.Name, f.Name))
			}
			defaultlang = f.I18nID
			defaulttranslations = f.Translations
		}
	}

	for _, lang := range i18nfiles {
		templateData := TemplateData{i18nids, lang.I18nID, lang.Translations}
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

	cmd := exec.Command("npx", "@tailwindcss/cli", "-i", "../src/css/main.css", "-o", "main.css")
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
			f, err := os.ReadFile("../i18n/" + f.Name())
			if err != nil {
				return nil, err
			}
			i18n := I18nFile{}
			yaml.Unmarshal(f, &i18n)
			i18nfiles = append(i18nfiles, i18n)
		}
	}
	return i18nfiles, nil
}

func renderSite(data TemplateData) error {
	builddir := "../build"
	if !data.Currentlang.Default {
		builddir += "/" + data.Currentlang.Path
	}
	err := os.MkdirAll(builddir, os.ModePerm)
	if err != nil {
		return err
	}

	tmpl, err := template.ParseGlob("../src/partials/*.html")
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
			fmt.Printf("rendering [%s] %s\n", data.Currentlang.Path, path)
			f, err := os.Create(strings.Replace(path, "../src/content", builddir, 1))
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
