package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const chapterHTML = `<?xml version="1.0"?><!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head>
<body id="b"><div><h1>Dois</h1><p>um &nbsp; texto</p><p>dois<br/>linhas</p><section><p>dentro</p></section><p>fim</p></div><div><p>outra</p></div></body></html>`

func TestKosyncPlace_APathOfNamesAndAPathOfPositionsAreTheSameElement(t *testing.T) {
	html, total, err := parseChapter([]byte(chapterHTML))
	if err != nil {
		t.Fatal(err)
	}
	for tail, want := range map[string]string{
		"body":                   "epubcfi(/6/10!/4)",
		"body/div":               "epubcfi(/6/10!/4/2)",
		"body/div/p":             "epubcfi(/6/10!/4/2/4)",
		"body/div/p[2]":          "epubcfi(/6/10!/4/2/6)",
		"body/div/section/p":     "epubcfi(/6/10!/4/2/8/2)",
		"body/div/p[3]/text().4": "epubcfi(/6/10!/4/2/10)",
		"body/div[2]/p/text().0": "epubcfi(/6/10!/4/4/2)",
		"body/div/p[2]/br":       "epubcfi(/6/10!/4/2/6/2)",
		"body/div/p[2]/text().3": "epubcfi(/6/10!/4/2/6)",
	} {
		got, progression, ok := cfiOfXPointer(html, total, 5, tail)
		if !ok || got != want || progression < 0 || progression > 1 {
			t.Errorf("%s: %q %v %v, want %s", tail, got, progression, ok, want)
		}
	}
	for _, tail := range []string{"", "p", "body/div/p[9]", "body/ul", "body/div/p[0]x"} {
		if got, _, ok := cfiOfXPointer(html, total, 5, tail); ok {
			t.Errorf("%q should lead nowhere, got %s", tail, got)
		}
	}
	// and back: the CFI names the same element
	for _, c := range []struct{ cfi, want string }{
		{"epubcfi(/6/10[chap]!/4/2/6/1:5)", "/body/DocFragment[5]/body/div/p[2]"},
		{"epubcfi(/6/10!/4/4/2)", "/body/DocFragment[5]/body/div[2]/p"},
		{"epubcfi(/6/10!/4/2/8/2)", "/body/DocFragment[5]/body/div/section/p"},
		{"epubcfi(/6/10!/4)", "/body/DocFragment[5]/body"},
	} {
		n, inside, ok := itemOfCFI(c.cfi)
		if !ok || n != 5 {
			t.Fatalf("%s: item %d %v", c.cfi, n, ok)
		}
		if got, ok := xpointerOfCFI(html, n, inside); !ok || got != c.want {
			t.Errorf("%s: %q %v, want %s", c.cfi, got, ok, c.want)
		}
	}
	for _, cfi := range []string{"", "epubcfi(/6/10)", "epubcfi(/6/3!/4)", "epubcfi(/6/10!/4/2/98)", "epubcfi(/6/10!/8)", "not a cfi"} {
		n, inside, ok := itemOfCFI(cfi)
		if ok {
			if got, ok2 := xpointerOfCFI(html, n, inside); ok2 {
				t.Errorf("%q should lead nowhere, got %s", cfi, got)
			}
		}
	}
}

func TestKosyncPlace_HowFarIntoTheChapterTheElementIs(t *testing.T) {
	html, total, _ := parseChapter([]byte(chapterHTML))
	_, early, _ := cfiOfXPointer(html, total, 1, "body/div/p")
	_, late, _ := cfiOfXPointer(html, total, 1, "body/div[2]/p")
	if !(early < late && late < 1) {
		t.Errorf("the first paragraph (%v) comes before the last (%v)", early, late)
	}
}

// What a Kindle sent (16/10/2026) for a real book, when it was on the 166th paragraph of the second chapter: the path leads to an element of
// that chapter, and back.
func TestKosyncPlace_WhatAKindleSentForARealBook(t *testing.T) {
	book := filepath.Join(os.Getenv("HOME"), "Downloads", "As foices Contos do Universo de Sycthe.epub")
	if _, err := os.Stat(book); err != nil {
		t.Skip("the book is not on this machine")
	}
	spine, err := epubSpine(book)
	if err != nil || len(spine) < 6 {
		t.Fatalf("spine: %v %d", err, len(spine))
	}
	html, total, err := readChapter(book, spine[5])
	if err != nil {
		t.Fatal(err)
	}
	cfi, progression, ok := cfiOfXPointer(html, total, 6, "body/div/p[166]/text().0")
	if !ok || !strings.HasPrefix(cfi, "epubcfi(/6/12!/4/2/") {
		t.Fatalf("cfi = %q %v", cfi, ok)
	}
	if progression < 0.5 || progression >= 1 {
		t.Errorf("166 of 181 paragraphs: %v into the chapter", progression)
	}
	n, inside, _ := itemOfCFI(cfi)
	if back, ok := xpointerOfCFI(html, n, inside); !ok || back != "/body/DocFragment[6]/body/div/p[166]" {
		t.Errorf("back = %q %v", back, ok)
	}
}
