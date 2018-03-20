/*
 * Generates a data set and saves it as a csv file
 */

package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
)

/* Config oauth1 */
var config *oauth1.Config
var token *oauth1.Token
var httpClient *http.Client

/* Config oauth2 */
// var config = &oauth2.Config{}
// var token = &oauth2.Token{AccessToken: access_token}
// var httpClient = config.Client(oauth2.NoContext, token)

/* Mode Variables */
var mode bool
var single_handle []string
var input_file string
var output_dir string
var count int
var ext bool
var cdir = false

/* Concurrent Variables */
var glb_batch_cnt = 0
var glb_batch_max = 0
var glb_sleep = false
var glb_current_time = time.Now()

var wg sync.WaitGroup
var glb_batch_mutex sync.Mutex
var glb_sleep_mutex sync.Mutex

const max_count = 900

/* Stores a Twitter Users */
type User struct {
	Name       string
	ScreenName string
}

/* Stores data that has been parsed from the Tweet Interface */
type CustomTweet struct {
	Date string
	Text string
	ID   int64
}

/* ---------------------------------------------------------------------------
 * -- MAIN -----------------------------------------------------------------*/
func main() {

	/* Grab command line flags */
	var file_mode string
	flag.StringVar(&file_mode, "f", "", "File Mode: Indicate a file to read Twitter handles.")

	var single_mode string
	flag.StringVar(&single_mode, "s", "", "Single Mode: Indicate a Twitter handle to mine.")

	var Foutput string
	flag.StringVar(&Foutput, "dir", "output/", "Specify the directory to store mined Tweets.")

	extendPtr := flag.Bool("e", false, "Run in extended mode?")
	envvarPtr := flag.Bool("r", false, "Used to reset API and Token information.")
	numPtr := flag.Int("n", 10, "The number of most recent Tweets to mine.")

	flag.Parse()

	if file_mode != "" && single_mode == "" {
		mode = true
		input_file = file_mode
	} else if single_mode != "" && file_mode == "" {
		mode = false
		single_handle = []string{single_mode}
	} else {
		print_usage(0)
		return
	}

	output_dir = Foutput
	ext = *extendPtr
	count = *numPtr

	system_init(*envvarPtr)

	fmt.Print("Mining Tweets...\n\n")
	start_time := time.Now()

	if ext {
		fmt.Println("Running in extended mode.")
		fmt.Println("Including retweets and replies in records.")
	} else {
		fmt.Println("Running in normal mode.")
		fmt.Println("Excluding retweets and replies in records.")
	}

	var user_list []User

	if mode {
		user_list = read()
	} else {
		user_list = []User{User{
			ScreenName: single_handle[0],
		}}
	}

	for _, user := range user_list {

		// glb_sleep_mutex.Lock()
		// current_sleep := glb_sleep
		// glb_sleep_mutex.Unlock()

		// if !current_sleep {
		wg.Add(1)
		go request(user)
		// }

	}

	/* Wait for all threads to terminate */
	wg.Wait()

	delta_time := time.Since(start_time)
	fmt.Println("\n...Completed in", delta_time, "and", glb_batch_max, "batches")

}

func request(user User) {
	tweet_list := process_tweets(user)
	save_set(user, tweet_list)
	wg.Done()
}

func read() []User {
	csvFile, _ := os.Open(input_file)
	reader := csv.NewReader(bufio.NewReader(csvFile))

	var users []User

	for {
		line, err := reader.Read()

		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Println("Error with: ", line[0])
			log.Fatal(err)
			panic(err)
		}

		new_user := User{
			Name:       line[0],
			ScreenName: line[1],
		}

		users = append(users, new_user)

	}

	// usersJson, _ := json.Marshal(users)
	// fmt.Println(string(usersJson))

	return users
}

func process_tweets(user User) []CustomTweet {
	var tweets []CustomTweet

	/* Helper functions */
	demux := twitter.NewSwitchDemux()

	demux.Tweet = func(tweet *twitter.Tweet) {

		/* Replace quotation marks with ~ */
		ptext := strings.Replace(tweet.FullText, "\u201C", "~", -1)
		ptext = strings.Replace(ptext, "\u201D", "~", -1)

		/* Replace carriage returns and new line characters with empty string */
		ptext = strings.Replace(ptext, "\n", "", -1)
		ptext = strings.Replace(ptext, "\r", "", -1)

		tweets = append(tweets, CustomTweet{
			ID:   tweet.ID,
			Date: tweet.CreatedAt,
			Text: ptext,
		})

	}
	/* End Helper functions */

	/* Have to grab tweets in batches of 200 */
	batch_count := count
	var request_count int
	if count < 200 {
		request_count = count
	} else {
		request_count = 200
	}

	/* Increment the glb_batch_mutex */
	glb_batch_mutex.Lock()
	glb_batch_cnt += int(math.Ceil(float64(count) / 200))

	/* Set up the client */
	client := twitter.NewClient(httpClient)

	/* Hit the max for a 15 minute window, check to see how long to rest */
	// if(glb_batch_cnt > max_count){
	// 	delta_time = time.Now() -
	// }

	glb_batch_max += int(math.Ceil(float64(count) / 200))
	glb_batch_mutex.Unlock()

	for batch_count > 0 {

		timeline_params := twitter.UserTimelineParams{
			ScreenName: user.ScreenName,
			Count:      request_count,
			TrimUser:   twitter.Bool(true),
			TweetMode:  "extended",
		}

		/* Calculate the starting point if running a subsequent batch */
		if batch_count != count {
			max_id := tweets[len(tweets)-1].ID - 1
			timeline_params.MaxID = max_id
		}

		if ext {
			timeline_params.ExcludeReplies = twitter.Bool(false)
			timeline_params.IncludeRetweets = twitter.Bool(true)
		} else {
			timeline_params.ExcludeReplies = twitter.Bool(true)
			timeline_params.IncludeRetweets = twitter.Bool(false)
		}

		/* Grab tweets of the specified user */
		tweet, resp, err := client.Timelines.UserTimeline(&timeline_params)

		/* Error and Response Handling */
		if err != nil {
			fmt.Println(err)
			fmt.Println("Error: ", user.Name)
		}
		if resp == nil {
			fmt.Println("Bad response: ", user.Name)
		}

		/* Process each tweet and add it to the list of tweets */
		for _, ptweet := range tweet {
			demux.Tweet(&ptweet)
		}

		/* Set up for the next batch */
		batch_count -= 200

		if batch_count < 200 {
			request_count = batch_count
		}
	}
	return tweets
}

func save_set(user User, list []CustomTweet) {

	/* Make sure the directory is created */
	if !cdir {
		CreateDir(output_dir)
		cdir = true
	}

	time := time.Now().Local()
	filename_array := []string{output_dir, "/", user.ScreenName, "_", time.Format("Jan022006"), ".csv"}
	file_name := strings.Join(filename_array, "")

	file, err := os.Create(file_name)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, value := range list {
		line_array := []string{value.Date, value.Text}

		err := writer.Write(line_array)
		if err != nil {
			log.Fatal(err)
		}
	}
}

/* Written by Siong-Ui Te, siongui.github.io */
func CreateDir(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			panic(err)
		}
	}
}

func print_usage(flag int) {
	if flag == 0 {
		println("Please choose either Single Mode (-s <TwitterHandle>) or File Mode (-f <CSVFile>)")
	}
}

func system_init(env bool) {

	conf, err := read_config()

	if env || err != nil {
		fmt.Println("TweetMine requires that the user create an App registered with Twitter.")
		fmt.Println("This can be done at https://apps.twitter.com/")

		fmt.Println("\nThis program saves several pieces of information regarding your Twitter app.")
		fmt.Println("This data should not be stored anywhere outside of the .tweetmine.ini")

		fmt.Print("\nTo reset environment variables in the future run with flag -env\n")
		fmt.Print("Otherwise they can be updated in the .ini file\n\n")

		var_alias := []string{"Consumer Key", "Consumer Secret", "Access Token", "Token Secret"}
		var_mapped := []string{"consumer_key", "consumer_secret", "access_token", "token_secret"}
		conf = make(map[string]string)

		for i, _ := range var_alias {
			var value string
			fmt.Print(var_alias[i], ": ")
			fmt.Scanln(&value)
			conf[var_mapped[i]] = value
		}
		save_config(conf)
	}

	config = oauth1.NewConfig(conf["consumer_key"], conf["consumer_secret"])
	token = oauth1.NewToken(conf["access_token"], conf["token_secret"])
	httpClient = config.Client(oauth1.NoContext, token)
}

func save_config(conf map[string]string) {

	file, err := os.Create(".tweetmine.ini")

	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for key, value := range conf {
		line := strings.Join([]string{key, value}, "=")
		fmt.Fprintln(writer, line)
	}
	writer.Flush()
}

func read_config() (map[string]string, error) {
	conf := make(map[string]string)

	file, err := os.Open(".tweetmine.ini")

	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		key_value := strings.Split(scanner.Text(), "=")
		conf[key_value[0]] = key_value[1]
	}
	return conf, nil
}
