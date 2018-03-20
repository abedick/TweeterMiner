# TweeterMiner

Quickly mine Tweets based on Twitter Handles. The user will need to create a Twitter app and supply the needed information on the first run. ( < ~1 min ). 


## Usage

`./TweetMiner <mode_flag> <mode_condition flag> <extended> -n <number_tweets> -dir <output_directory>`

## Flags

#### Mode Flag

In single use mode, a single Twitter Handle must be specified and only data from this handle will be mined.

> `-s <TwitterHandle>`


In file use mode, a CSV file must be specified in the format: `<DisplayName>,<TwitterHandle>`.

> `-f <CSV_File>` 


#### Extended Mode

`-e` specifies the program will run in extended mode. Extended mode returns Tweets that are Retweets and Replies. In nonextended mode, only stand alone Tweets will be saved. (This is good for analyzing things such as speech patterns, etc.)

#### Number of Files

`-n <int>` specifies the number of newest Tweets to grab per Twitter handle. The __maximum may or may not be 3200__ according to some versions of the Twitter API. __There is a hard 900 requests per 15 minutes when using the API. Each request can return 200 Tweets at maximum therefore when using a large n, the program will pause to wait for more requests to be made.__


#### Output Directory

Specify a directory to output the files to. One CSV file will be generated per successful Tweet mine in the format of: 

`<TwitterHandle>_<DateToday>.csv`

#### Reset Settings

`-r` will force the user to reneter their Twitter API information. __Note*__ This information can also be modified in .tweetmine.ini. 

## Dependencies

* github.com/dghubble/go-twitter/twitter
* github.com/dghubble/oauth1
___

Tested with Go version: go1.8.3 linum/amd64.

Tested on Unbuntu 16.10