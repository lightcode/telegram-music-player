package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/analysis/char/asciifolding"
	"github.com/blevesearch/bleve/analysis/token/lowercase"
	"github.com/blevesearch/bleve/analysis/tokenizer/unicode"
	"github.com/fhs/gompd/mpd"
	log "github.com/sirupsen/logrus"
)

const defaultMPDHost = "localhost:6600"

type Music struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Filename string `json:"filename"`
}

type MusicPlayer struct {
	client *mpd.Client
	index  bleve.Index
	musics map[string]Music
}

func NewMusicPlayer() *MusicPlayer {
	client, err := mpd.Dial("tcp", defaultMPDHost)
	if err != nil {
		log.Fatalf("cannot connect to MPD: %s", err)
	}

	p := &MusicPlayer{
		client: client,
	}
	go p.pingMPD()
	return p
}

func (p *MusicPlayer) pingMPD() {
	for {
		err := p.client.Ping()
		if err != nil {
			log.Errorf("cannot connect to MPD: %s", err)
		}
		time.Sleep(15 * time.Second)
	}
}

func (p *MusicPlayer) GetMusic(id string) Music {
	return p.musics[id]
}

func (p *MusicPlayer) InitIndex() {
	p.musics = make(map[string]Music)

	attrs, err := p.client.ListAllInfo("/")
	if err != nil {
		log.Errorf("cannot list all client info: %v", err)
	}

	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultAnalyzer = "musicTitle"

	err = indexMapping.AddCustomAnalyzer("musicTitle",
		map[string]interface{}{
			"type":      custom.Name,
			"tokenizer": unicode.Name,
			"char_filters": []string{
				asciifolding.Name,
			},
			"token_filters": []string{
				lowercase.Name,
			},
		})
	if err != nil {
		log.Errorf("Failed to add customAnalyser: %v", err)
	}

	index, err := bleve.NewMemOnly(indexMapping)
	if err != nil {
		panic(err)
	}
	for i, attr := range attrs {
		m := Music{
			ID:       fmt.Sprintf("m-%d", i),
			Title:    attr["Title"],
			Filename: attr["file"],
		}
		if err := index.Index(m.ID, m); err != nil {
			log.Errorf("Failed to index %s", m.Filename)
		}
		p.musics[m.ID] = m
	}

	p.index = index
}

func (p *MusicPlayer) PlayFile(filename string) error {
	log.Infof("Play %s", filename)
	id, err := p.client.AddId(filename, -1)
	if err != nil {
		return err
	}
	return p.client.PlayId(id)
}

func (p *MusicPlayer) Play() error {
	return p.client.Pause(false)
}

func (p *MusicPlayer) Pause() error {
	return p.client.Pause(true)
}

func (p *MusicPlayer) Next() error {
	return p.client.Next()
}

func (p *MusicPlayer) Previous() error {
	return p.client.Previous()
}

func (p *MusicPlayer) GetVolume() (int, error) {
	status, err := p.client.Status()
	if err != nil {
		return -1, err
	}
	volume, err := strconv.Atoi(status["volume"])
	if err != nil {
		return -1, err
	}
	return volume, nil
}

func (p *MusicPlayer) SetVolume(vol int) error {
	return p.client.SetVolume(vol)
}

func (p *MusicPlayer) Search(text string) (*bleve.SearchResult, error) {
	query := bleve.NewQueryStringQuery(text)
	searchRequest := bleve.NewSearchRequest(query)
	return p.index.Search(searchRequest)
}
