package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	tb "gopkg.in/tucnak/telebot.v2"
)

type ChatBot struct {
	mp  *MusicPlayer
	bot *tb.Bot
}

func NewChatBot() *ChatBot {
	musicPlayer := NewMusicPlayer()
	musicPlayer.InitIndex()

	b, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv("TELEGRAM_TOKEN"),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return &ChatBot{
		mp:  musicPlayer,
		bot: b,
	}
}

func (cb *ChatBot) Run() {
	err := cb.bot.SetCommands([]tb.Command{
		tb.Command{Text: "play", Description: "Start playing"},
		tb.Command{Text: "pause", Description: "Pause the music"},
		tb.Command{Text: "next", Description: "Next music"},
		tb.Command{Text: "prev", Description: "Previous music"},
		tb.Command{Text: "volume", Description: "Show or set the volume"},
	})
	if err != nil {
		log.Errorf("Failed to set commands: %v", err)
	}

	cb.bot.Handle("/next", cb.cmdNext)
	cb.bot.Handle("/pause", cb.cmdPause)
	cb.bot.Handle("/play", cb.cmdPlay)
	cb.bot.Handle("/prev", cb.cmdPrev)
	cb.bot.Handle("/volume", cb.cmdVolume)
	cb.bot.Start()
}

func (cb *ChatBot) sendCurrentVolume(m *tb.Message) {
	var (
		selector = &tb.ReplyMarkup{}
		btnUp    = selector.Data("UP", "btn_volume", "up")
		btnDown  = selector.Data("DOWN", "btn_volume", "down")
	)

	selector.Inline(selector.Row(btnUp, btnDown))

	cb.bot.Handle(&btnUp, func(c *tb.Callback) {
		inc := 0
		switch c.Data {
		case "up":
			inc = 2
		case "down":
			inc = -2
		}
		vol, err := cb.mp.GetVolume()
		vol += inc
		if err != nil {
			log.Errorf("Failed to get volume: %v", err)
			if err := cb.bot.Respond(c); err != nil {
				log.Errorf("Failed to respond to client: %v", err)
			}
		}
		vol = int(math.Min(math.Max(float64(vol), 0), 100))
		if err := cb.mp.SetVolume(vol); err != nil {
			log.Errorf("Failed to set volume: %v", err)
		}
		_, err = cb.bot.Edit(c.Message, fmt.Sprintf("Current volume is: %d", vol), selector)
		if err != nil {
			log.Errorf("Failed to edit message: %v", err)
		}
		if err := cb.bot.Respond(c); err != nil {
			log.Errorf("Failed to respond to client: %v", err)
		}
	})

	vol, err := cb.mp.GetVolume()
	if err != nil {
		cb.actionErrorf(m, "Failed to get volume: %v", err)
	}
	cb.sendMessage(m.Sender, fmt.Sprintf("Current volume is: %d", vol), selector)
}

func (cb *ChatBot) cmdVolume(m *tb.Message) {
	if m.Payload == "" {
		cb.sendCurrentVolume(m)
		return
	}

	newVol, err := strconv.Atoi(m.Payload)
	if err != nil {
		cb.sendMessage(m.Sender, "The volume should be a number.")
		return
	}

	if newVol < 0 || newVol > 100 {
		cb.sendMessage(m.Sender, "The volume should be beetween 0 and 100.")
	}

	if err = cb.mp.SetVolume(newVol); err != nil {
		log.Errorf("Failed to set volume: %v", err)
	}
}

func (cb *ChatBot) cmdPause(m *tb.Message) {
	if err := cb.mp.Pause(); err != nil {
		cb.actionErrorf(m, "Failed to run command pause: %v", err)
	}
}

func (cb *ChatBot) cmdPlay(m *tb.Message) {
	if m.Payload == "" {
		if err := cb.mp.Play(); err != nil {
			cb.actionErrorf(m, "Failed to run command play: %v", err)
			return
		}
		cb.sendCurrentMusicName(m)
		return
	}

	cb.findAndPlay(m)
}

func (cb *ChatBot) cmdNext(m *tb.Message) {
	if err := cb.mp.Next(); err != nil {
		cb.actionErrorf(m, "Failed to run command next: %v", err)
	} else {
		cb.sendCurrentMusicName(m)
	}
}

func (cb *ChatBot) cmdPrev(m *tb.Message) {
	if err := cb.mp.Previous(); err != nil {
		cb.actionErrorf(m, "Failed to run command previous: %v", err)
	} else {
		cb.sendCurrentMusicName(m)
	}
}

// actoinError log the error and notify the client
func (cb *ChatBot) actionErrorf(m *tb.Message, format string, args ...interface{}) {
	log.Errorf(format, args...)
	cb.sendMessage(m.Sender, "Your command returns an error.")
}

func (cb *ChatBot) sendMessage(to tb.Recipient, what interface{}, options ...interface{}) *tb.Message {
	msg, err := cb.bot.Send(to, what, options...)
	if err != nil {
		log.Errorf("Failed to send message: %v", err)
	}
	return msg
}

func (cb *ChatBot) sendCurrentMusicName(m *tb.Message) {
	if attrs, err := cb.mp.client.CurrentSong(); err == nil {
		cb.sendMessage(m.Sender, fmt.Sprintf("Now playing %s", attrs["Title"]))
	}
}

func (cb *ChatBot) findAndPlay(m *tb.Message) {
	searchResult, err := cb.mp.Search(m.Payload)
	if err != nil {
		cb.actionErrorf(m, "Search failed: %v", err)
	}

	if len(searchResult.Hits) == 0 {
		cb.sendMessage(m.Sender, "No music found.")
		return
	}

	menu := &tb.ReplyMarkup{}
	buttons := make([]tb.Btn, len(searchResult.Hits))
	for i := 0; i < len(searchResult.Hits); i++ {
		fileid := searchResult.Hits[i].ID
		buttons[i] = menu.Data(fmt.Sprintf("%d", i+1), "btn_change_music", fileid)
	}

	cb.bot.Handle(&buttons[0], func(c *tb.Callback) {
		music := cb.mp.GetMusic(c.Data)
		err := cb.mp.PlayFile(music.Filename)
		if err != nil {
			cb.actionErrorf(m, "Failed to play %s: %v", music.Filename, err)
		}
		if err := cb.bot.Respond(c); err != nil {
			log.Errorf("Failed to respond to client: %v", err)
		}
	})

	maxRowSize := 5
	count := len(searchResult.Hits)
	rowCount := int(math.Ceil(float64(count) / float64(maxRowSize)))
	rows := make([]tb.Row, rowCount)

	for r := 0; r < rowCount; r++ {
		rowSize := int(math.Min(float64(maxRowSize), float64(count-(r*maxRowSize))))
		row := make([]tb.Btn, rowSize)
		for j := 0; j < rowSize; j++ {
			row[j] = buttons[r*maxRowSize+j]
		}
		rows[r] = menu.Row(row...)
	}

	menu.Inline(rows...)

	message := ""
	for i, hit := range searchResult.Hits {
		message += fmt.Sprintf("%d. %s\n", i+1, cb.mp.GetMusic(hit.ID).Title)
	}

	cb.sendMessage(m.Sender, message, menu)
}
