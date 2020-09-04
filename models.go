package main

import (
	"fmt"
	"net/url"
)

type login struct {
	Remember bool `json:"remember"`
	Server   struct {
		Address    string      `json:"address"`
		Port       int         `json:"port"`
		ServerID   int         `json:"server_id"`
		Population int         `json:"population"`
		Tournament interface{} `json:"tournament"`
		GuildID    int         `json:"guild_id"`
		ServerName string      `json:"server_name"`
		Status     string      `json:"status"`
		HappyHour  int         `json:"happy_hour"`
	} `json:"server"`
	Domain   string `json:"domain"`
	LoginID  int    `json:"login_id"`
	UserPass string `json:"user_pass"`
	UserName string `json:"user_name"`
	Build    string `json:"build"`
}

type playerInfo struct {
	Success      bool        `json:"success,omitempty"`
	Rank         int         `json:"rank"`
	Hats         int         `json:"hats"`
	Group        string      `json:"group"`
	Friend       int         `json:"friend"`
	Ignored      int         `json:"ignored"`
	Status       string      `json:"status"`
	LoginDate    string      `json:"loginDate"`
	RegisterDate string      `json:"registerDate"`
	Hat          string      `json:"hat"`
	Head         string      `json:"head"`
	Body         string      `json:"body"`
	Feet         string      `json:"feet"`
	HatColor     string      `json:"hatColor"`
	HeadColor    string      `json:"headColor"`
	BodyColor    string      `json:"bodyColor"`
	FeetColor    string      `json:"feetColor"`
	GuildID      string      `json:"guildId"`
	GuildName    string      `json:"guildName"`
	Name         string      `json:"name"`
	UserID       string      `json:"userId"`
	HatColor2    interface{} `json:"hatColor2"`
	HeadColor2   interface{} `json:"headColor2"`
	BodyColor2   interface{} `json:"bodyColor2"`
	FeetColor2   interface{} `json:"feetColor2"`
	ExpPoints    string      `json:"exp_points"`
	ExpToRank    int         `json:"exp_to_rank"`
}

func getPlayerInfo(name string) (*playerInfo, error) {
	u := fmt.Sprintf("https://pr2hub.com/get_player_info.php?name=%s", url.QueryEscape(name))
	var p playerInfo
	return &p, httpgetjson(u, &p)
}

type Levels struct {
	Levels []struct {
		LevelID   int     `json:"level_id"`
		Version   int     `json:"version"`
		Title     string  `json:"title"`
		Rating    float64 `json:"rating"`
		PlayCount int     `json:"play_count"`
		MinLevel  int     `json:"min_level"`
		Note      string  `json:"note"`
		Live      int     `json:"live"`
		Pass      bool    `json:"pass"`
		Type      string  `json:"type"`
		Time      int     `json:"time"`
		UserName  string  `json:"user_name"`
		UserGroup string  `json:"user_group"`
	} `json:"levels"`
	Hash string `json:"hash"`
}
