/*
 * TweeterMiner
 *
 * File		: main.go
 *
 * Author	: Abe Dick <abedick8213(at)gmail.com>
 * Date		: March 2018
 *
 * Desc		: Uses the Twitter API to create data sets of a Twitter user's
 *			  most recent Tweets. Tweets are then stored in CSV format.
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

/* Concurrent Variables */
var glb_batch_cnt = 0
var glb_batch_max = 0
var glb_sleep = false
var glb_current_time = time.Now()

var glb_batch_mutex sync.Mutex
var glb_sleep_mutex sync.Mutex

const max_count = 900

/* Stores envirment runtime variables */
type Config struct {
	n int

	mode          bool
	extended      bool
	output_path   string
	input_file    string
	single_handle []string

	/* API Config */
	config     *oauth1.Config
	token      *oauth1.Token
	httpClient *http.Client
	client     *twitter.Client

	/* Concurrent */
	wg sync.WaitGroup
}

var glb_config Config

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

	/* Set up environment */
	system_init()

	/* Start */
	fmt.Print("Mining Tweets...\n\n")
	start_time := time.Now()

	if glb_config.extended {
		fmt.Println("Running in extended mode.")
		fmt.Println("Including retweets and replies in records.")
	} else {
		fmt.Println("Running in normal mode.")
		fmt.Println("Excluding retweets and replies in records.")
	}

	/*
	 * Populate the users list
	 */
	var user_list []User

	if glb_config.mode {
		user_list = read()
	} else {
		user_list = []User{User{
			ScreenName: glb_config.single_handle[0],
		}}
	}

	/*
	 * Start a new goroutine for each user
	 */
	for _, user := range user_list {

		// glb_sleep_mutex.Lock()
		// current_sleep := glb_sleep
		// glb_sleep_mutex.Unlock()

		// if !current_sleep {
		glb_config.wg.Add(1)
		go request(user)
		// }

	}

	/* Wait for all threads to terminate */
	glb_config.wg.Wait()

	/* Report */
	delta_time := time.Since(start_time)
	fmt.Println("\n...Completed in", delta_time, "and", glb_batch_max, "batches")

}

/*
 * Request
 *
 * @param user: A well defined User structure
 *
 * -> Grab list of processed tweets from process_tweets()
 * -> Save the Tweets to file with save_set()
 *
 * Designed to work concurrently using goroutines to be able to process
 * many users each using their own thread.
 */
func request(user User) {

	tweet_list := process_tweets(user)
	save_set(user, tweet_list)

	glb_config.wg.Done()
}

/*
 * Read
 *
 * Returns: User struct array populated from the glb_config input file.glb_config.
 * 			Else, error.
 */
func read() []User {
	csvFile, _ := os.Open(glb_config.input_file)
	reader := csv.NewReader(bufio.NewReader(csvFile))

	var users []User

	for {
		line, err := reader.Read()

		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Error in CSV input file")
			panic(err)
		}

		new_user := User{
			Name:       line[0],
			ScreenName: line[1],
		}

		users = append(users, new_user)
	}
	return users
}

/*
 * Process Tweets
 *
 * @param: user: A well defined User structure
 *
 * Returns: List of processed Tweets from the Twitter API
 *
 * Attepts to interact with Twitter API to grab some number of tweets from
 * a calculated starting point. While working, checks to see if request can be
 * made within the 15 minute per 900 request window as dictated by Twitter.
 */
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
	batch_count := glb_config.n
	var request_count int
	if glb_config.n < 200 {
		request_count = glb_config.n
	} else {
		request_count = 200
	}

	/* Increment the glb_batch_mutex */
	glb_batch_mutex.Lock()
	glb_batch_cnt += int(math.Ceil(float64(glb_config.n) / 200))

	/* Hit the max for a 15 minute window, check to see how long to rest */
	// if(glb_batch_cnt > max_count){
	// 	delta_time = time.Now() -
	// }

	glb_batch_max += int(math.Ceil(float64(glb_config.n) / 200))
	glb_batch_mutex.Unlock()

	for batch_count > 0 {

		timeline_params := twitter.UserTimelineParams{
			ScreenName: user.ScreenName,
			Count:      request_count,
			TrimUser:   twitter.Bool(true),
			TweetMode:  "extended",
		}

		/* Calculate the starting point if running a subsequent batch */
		if batch_count != glb_config.n {
			max_id := tweets[len(tweets)-1].ID - 1
			timeline_params.MaxID = max_id
		}

		if glb_config.extended {
			timeline_params.ExcludeReplies = twitter.Bool(false)
			timeline_params.IncludeRetweets = twitter.Bool(true)
		} else {
			timeline_params.ExcludeReplies = twitter.Bool(true)
			timeline_params.IncludeRetweets = twitter.Bool(false)
		}

		/* Grab tweets of the specified user */
		tweet, resp, err := glb_config.client.Timelines.UserTimeline(&timeline_params)

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

/*
 * Save Set
 *
 * @param: user, a well defined User structure
 * @param: list, a slice of CustomTweet structures, all well defined
 *
 * Creates a file and outputs list in CSV format.
 */
func save_set(user User, list []CustomTweet) {

	time := time.Now().Local()
	filename_array := []string{glb_config.output_path, "/", user.ScreenName, "_", time.Format("Jan022006"), ".csv"}
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

/*
 * System Init
 *
 * Sets parameters in glb_config and the api settings file .tweetmine.ini.
 */
func system_init() {

	/*
	 * Set up and parse command line arguments
	 */
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

	glb_config.output_path = Foutput
	glb_config.extended = *extendPtr
	glb_config.n = *numPtr

	if file_mode != "" && single_mode == "" {
		glb_config.mode = true
		glb_config.input_file = file_mode
	} else if single_mode != "" && file_mode == "" {
		glb_config.mode = false
		glb_config.single_handle = []string{single_mode}
	} else {
		print_usage(0)
		return
	}

	/* Config the output directory */
	CreateDir(glb_config.output_path)

	/*
	 * Set up the Twitter API connection
	 */
	api_conf, err := read_config()

	if *envvarPtr || err != nil {
		fmt.Println("TweetMine requires that the user create an App registered with Twitter.")
		fmt.Println("This can be done at https://apps.twitter.com/")

		fmt.Println("\nThis program saves several pieces of information regarding your Twitter app.")
		fmt.Println("This data should not be stored anywhere outside of the .tweetmine.ini")

		fmt.Print("\nTo reset API and token information run TweeterMiner with -r\n")
		fmt.Print("Otherwise they can be updated in the .ini file\n\n")

		var_alias := []string{"Consumer Key", "Consumer Secret", "Access Token", "Token Secret"}
		var_mapped := []string{"consumer_key", "consumer_secret", "access_token", "token_secret"}
		api_conf = make(map[string]string)

		for i, _ := range var_alias {
			var value string
			fmt.Print(var_alias[i], ": ")
			fmt.Scanln(&value)
			api_conf[var_mapped[i]] = value
		}
		save_config(api_conf)
	}

	/* Config oauth1 */
	glb_config.config = oauth1.NewConfig(api_conf["consumer_key"], api_conf["consumer_secret"])
	glb_config.token = oauth1.NewToken(api_conf["access_token"], api_conf["token_secret"])
	glb_config.httpClient = glb_config.config.Client(oauth1.NoContext, glb_config.token)
	glb_config.client = twitter.NewClient(glb_config.httpClient)

	/* Config oauth2 */
	// var config = &oauth2.Config{}
	// var token = &oauth2.Token{AccessToken: access_token}
	// var httpClient = config.Client(oauth2.NoContext, token)
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
