package randomnamegenerator

import (
	"fmt"
	"math/rand"
	"time"
)

var (
	adjectives = []string{
		"pink",
		"red",
		"orange",
		"yellow",
		"green",
		"blue",
		"purple",
		"white",
		"violet",
		"black",
		"gray",
		"silver",
		"chrome",
		"gold",
		"carbon",
		"fast",
		"speedy",
		"comfortable",
		"quick",
		"swift",
		"rapid",
		"turbo",
		"sporty",
		"blistering",
		"hasty",
		"zippy",
		"supersonic",
		"steady",
		"unshakable",
		"nitro",
		"muscular",
		"sturdy",
		"robust",
		"solid",
		"beefy",
		"forceful",
		"mighty",
		"intense",
		"fierce",
		"strong",
		"overwhelming",
		"dominant",
		"formidable",
		"tenacious",
		"vigorous",
		"wicked",
		"sublime",
		"confident",
		"grandeur",
		"majestic",
		"glorious",
		"grand",
		"jolly-good",
		"grand-theft",
	}

	nouns = []string{
		"auto",
		"warrior",
		"racer",
		"wheels",
		"pup",
		"meteor",
		"engine",
		"tank",
		"motor",
		"wagon",
		"comet",
		"bolt",
		"rascal",
		"trigger",
		"beast",
		"ogre",
		"horsepower",
		"rottweiler",
		"cheetah",
		"dagger",
		"bullet",
		"missile",
		"tornado",
		"box",
		"moustache",
		"rambo",
		"vroom",
		"fireball",
		"ghost",
		"speeder",
		"mothership",
		"stallion",
		"trucker",
		"beauty",
		"exploder",
		"tires",
		"baby",
		"intercooler",
		"tachyon",
		"frog",
		"firefly",
		"wave",
		"lightning",
		"thunder",
		"kings-landing",
		"winterfell",
		"white-harbor",
		"riverrun",
		"harrenhal",
		"lannisport",
		"casterly-rock",
		"kingswood",
		"blackwater-bay",
		"dragonstone",
		"highgarden",
		"storms-end",
	}
)

func Generate(suffix string) string {
	adjectiveIndex := rand.Intn(len(adjectives))
	nounIndex := rand.Intn(len(nouns))
	randomInt := (int(rand.Float32()+0.5) * 90) + 10

	return fmt.Sprintf("%s-%s-%s-%d", adjectives[adjectiveIndex], nouns[nounIndex], suffix, randomInt)
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}
