package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type answer struct {
	E struct {
		AnswerMessage string `json:"AnswerMessage"`
	} `json:"e"`
}

type videoData struct {
	InitSegment string `json:"init_segment"`
	BaseUrl     string `json:"base_url"`
	Bitrate     int    `json:"bitrate"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Segments    []struct {
		Url string `json:"url"`
	} `json:"segments"`
	AudioProvenance int `json:"AudioProvenance"`
}

type audioData struct {
	BaseUrl     string `json:"base_url"`
	Bitrate     int    `json:"bitrate"`
	InitSegment string `json:"init_segment"`
	Segments    []struct {
		Url string `json:"url"`
	} `json:"segments"`
}

type vimeoVideoData struct {
	BaseUrl string      `json:"base_url"`
	Video   []videoData `json:"video"`
	Audio   []audioData `json:"audio"`
}

type soundCloudResolve struct {
	Media struct {
		Transcodings []struct {
			Url string `json:"url"`
		} `json:"transcodings"`
	} `json:"media"`
}

type m3u8PlaylistUrl struct {
	Url string `json:"url"`
}

var client = &http.Client{}

const (
	cookie        string = "[[cookie]]"
	clientId      string = "[[clientId]]"
	coursesDir    string = "Courses/"
	maxGoroutines int    = 20
)

var (
	regexAdd              = regexp.MustCompile(`<form action="(.+?)" method="post">[\s\S]+?value="(.+?)"[\s\S]+?value="(.+?)"[\s\S]+?</form>`)
	regexCourse           = regexp.MustCompile(`<li class="item ttmik-courses-item">[\s\S]+?href="(.+?)"[\s\S]+?</li>`)
	regexCourseContent    = regexp.MustCompile(`<a class="ld-item-name ld-primary-color-hover" href="(.+?)">`)
	regexCourseName       = regexp.MustCompile(`<h1 class="entry-title">([\s\S]+?)</h1>`)
	regexLessonName       = regexp.MustCompile(`<div class="ld-focus-content">[\s\S]+?<h1>([\s\S]+?)</h1>`)
	regexIframe           = regexp.MustCompile(`<iframe[\s\S]+?src="(.+?)"[\s\S]+?</iframe>`)
	regexTrackNumber      = regexp.MustCompile(`tracks(?:/|%2F)(\d+)`)
	regexSecretToken      = regexp.MustCompile(`secret_token.+?&`)
	regexVimeoVideoLink   = regexp.MustCompile(`(https://player\.vimeo\.com/video/\d+?)\?`)
	regexAvcUrl           = regexp.MustCompile(`"avc_url":"(.+?)"`)
	regexLdText           = regexp.MustCompile(`<span class="ld-text">([\s\S]+?)</span>`)
	regexGetLessonContent = regexp.MustCompile(`<div class="ld-tab-content tab-content-lesson ld-visible" data-tab="lesson">([\s\S]+?)</div> <!-- lesson-tab-->`)
	regexContentCustom1   = regexp.MustCompile(`div class="ld-tab-content tab-content-custom1" data-tab="custom1">([\s\S]+?)</div>
 +?<div class="ld-tab-content tab-content-custom2" data-tab="custom2">`)
	regexContentCustom2 = regexp.MustCompile(`<div class="ld-tab-content tab-content-custom2" data-tab="custom2">([\s\S]+?)</div>
\s+?<div class="ld-tab-content tab-content-custom3" data-tab="custom3">`)
	regexContentCustom3 = regexp.MustCompile(`<div class="ld-tab-content tab-content-custom3" data-tab="custom3">([\s\S]+?)</div>
\s+?</div> <!--/.ld-tabs-content-->`)
	regexLessonHaveTest = regexp.MustCompile(`wpProQuizFront\(\{`)
	regexQuiz           = regexp.MustCompile(`quiz: (\d+?),`)
	regexQuizId         = regexp.MustCompile(`quizId: (\d+?),`)
	regexQuizNonce      = regexp.MustCompile(`quiz_nonce: '(.+?)',`)
	regexQuizJson       = regexp.MustCompile(`json: (\{[\s\S]+?}}),`)
	regexQuizQuestion   = regexp.MustCompile(`<div class="wpProQuiz_question_text">([\s\S]+?)</div>`)
	regexJustAMoment    = regexp.MustCompile(`<title>Just a moment\.\.\.</title>`)
)

func main() {
	request, err := http.NewRequest("GET", "https://talktomeinkorean.com/curriculum/", nil)
	if err != nil {
		panic(err)
	}
	request.Header.Set("cookie", cookie)

	response, err := client.Do(request)
	if err != nil {
		panic(err)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}

	html := string(body)
	// check if user is logged
	if !strings.Contains(html, "[[your user name]]") {
		log.Fatal("user is not logged")
	}
	// add all courses to learning center
	for _, match := range regexAdd.FindAllStringSubmatch(html, -1) {
		request, err = http.NewRequest(
			"POST",
			match[1],
			strings.NewReader(
				url.Values{
					"course_id":   {match[2]},
					"course_join": {match[3]},
				}.Encode(),
			),
		)
		if err != nil {
			panic(err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("cookie", cookie)
		_, err = client.Do(request)
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("completed adding all courses to learning center")

	// list courses
	for _, match := range regexCourse.FindAllStringSubmatch(html, -1)[:30] {
		request, err = http.NewRequest("GET", match[1], nil)
		if err != nil {
			panic(err)
		}
		request.Header.Set("cookie", cookie)
		response, err = client.Do(request)
		if err != nil {
			panic(err)
		}
		body, err = io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		err = response.Body.Close()
		if err != nil {
			panic(err)
		}

		html = string(body)
		// Course path
		coursePath := coursesDir + strings.NewReplacer(
			`&#8211;`, "-",
			`&amp;`, "&",
			`&#038;`, "&",
			`&#8217;`, "'",
			"/", "|",
			" ", "_",
		).Replace(strings.TrimSpace(regexCourseName.FindStringSubmatch(html)[1])) + "/"

		// list lessons
		var wg sync.WaitGroup
		guard := make(chan struct{}, maxGoroutines) // capped number of concurrent lessons
		for _, lesson := range regexCourseContent.FindAllStringSubmatch(html, -1) {
			guard <- struct{}{}
			wg.Add(1)
			go getLesson(lesson[1], coursePath, &wg, guard)
		}
		wg.Wait()
		fmt.Println(coursePath)
	}
}

func getLesson(lesson, coursePath string, wg *sync.WaitGroup, guard <-chan struct{}) {
	error524count := 0
	defer wg.Done()
	defer func() {
		<-guard
	}()
start:
	request, err := http.NewRequest("GET", lesson, nil)
	if err != nil {
		panic(err)
	}
	request.Header.Set("cookie", cookie)
	response, err := client.Do(request)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}
	// list all lesson's <span class="ld-text">[lesson fragments]</span>[:len(lessonFragemnts)-2]
	html := string(body)
	if html == "error code: 524" {
		error524count++
		if error524count < 2 {
			time.Sleep(7 * time.Second)
			goto start
		}
		panic("kurwa 524")
	}

	lessonPath := coursePath + strings.NewReplacer(
		`&#8230;`, `…`,
		`&#8217;`, "'",
		`&#038;`, "&",
		"/", "|",
		" ", "_",
		"&#8211;", "–",
		"&#8221;", "”",
		"&#8220;", "“",
		"&#8216;", "‘",
	).Replace(strings.TrimSpace(regexLessonName.FindStringSubmatch(html)[1])) + "/"

	err = os.MkdirAll(lessonPath, 0750)
	if err != nil {
		panic(err)
	}

	ldTexts := regexLdText.FindAllStringSubmatch(html, -1)
	lessonParts := make([]string, 0, len(ldTexts))
	for _, ldText := range ldTexts {
		lessonParts = append(lessonParts, "<h1>"+strings.TrimSpace(ldText[1])+"</h1>")
	}

	// get Answers for quiz if lesson have it
	if i := 0; "" != regexLessonHaveTest.FindString(html) {
	try:
		quizNonce := regexQuizNonce.FindStringSubmatch(html)[1]
		quizId := regexQuizId.FindStringSubmatch(html)[1]
		quiz := regexQuiz.FindStringSubmatch(html)[1]
		quizJson := regexQuizJson.FindStringSubmatch(html)[1]
		request, err = http.NewRequest(
			"POST",
			"https://talktomeinkorean.com/wp-admin/admin-ajax.php",
			strings.NewReader(
				url.Values{
					"action":           {"ld_adv_quiz_pro_ajax"},
					"func":             {"checkAnswers"},
					"data[quizId]":     {quizId},
					"data[quiz]":       {quiz},
					"data[course_id]":  {"0"},
					"data[quiz_nonce]": {quizNonce},
					"data[responses]":  {quizJson},
					"quiz":             {quiz},
					"course_id":        {"0"},
					"quiz_nonce":       {quizNonce},
				}.Encode(),
			),
		)
		if err != nil {
			panic(err)
		}
		request.Header.Set("cookie", cookie)
		request.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
		response, err = client.Do(request)
		if err != nil {
			panic(err)
		}
		body, err = io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		err = request.Body.Close()
		if err != nil {
			panic(err)
		}

		var answers map[string]interface{}

		err = json.Unmarshal(body, &answers)
		if err != nil {
			i++
			if i < 2 {
				time.Sleep(7 * time.Second)
				goto try
			}
			panic(err)
		}

		keys := make([]string, 0, len(answers))
		for k := range answers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		file, err := os.Create(lessonPath + "answers.md")
		if err != nil {
			panic(err)
		}

		for i, key := range keys {
			jsonStr, err := json.Marshal(answers[key])
			if err != nil {
				panic(err)
			}

			var ans answer
			err = json.Unmarshal(jsonStr, &ans)
			if err != nil {
				panic(err)
			}

			_, err = file.WriteString(fmt.Sprintf("<h1>Question %d</h1>\n%s\n", i+1, ans.E.AnswerMessage))
		}
		err = file.Close()
		if err != nil {
			panic(err)
		}
	}

	// get lesson's all text content
	lessonContent := strings.TrimSpace(regexGetLessonContent.FindStringSubmatch(html)[1])
	contentCustom1 := strings.TrimSpace(regexContentCustom1.FindStringSubmatch(html)[1])
	contentCustom2 := regexQuizQuestion.FindAllStringSubmatch(strings.TrimSpace(regexContentCustom2.FindStringSubmatch(html)[1]), -1)
	contentCustom3 := strings.TrimSpace(regexContentCustom3.FindStringSubmatch(html)[1])

	var quizContent string
	for i, question := range contentCustom2 {
		// questionContent := add downloading mp3 for question :)
		quizContent += fmt.Sprintf("<h3>Question %d</h3>\n%s\n", i+1, strings.TrimSpace(question[1]))
	}

	file, err := os.Create(lessonPath + "contents.md")
	if err != nil {
		panic(err)
	}
	_, err = file.WriteString(lessonParts[0] + "\n" + lessonContent + "\n")
	if err != nil {
		panic(err)
	}
	kurwaPart := 1
	if contentCustom1 != "" {
		_, err = file.WriteString(lessonParts[kurwaPart] + "\n" + contentCustom1 + "\n")
		if err != nil {
			panic(err)
		}
		kurwaPart++
	}
	if quizContent != "" {
		_, err = file.WriteString(lessonParts[kurwaPart] + "\n" + quizContent + "\n")
		if err != nil {
			panic(err)
		}
		kurwaPart++
	}
	if contentCustom3 != "" {
		_, err = file.WriteString(lessonParts[kurwaPart] + "\n" + contentCustom3 + "\n")
		if err != nil {
			panic(err)
		}
		kurwaPart++
	}
	err = file.Close()
	if err != nil {
		panic(err)
	}

	// list iframes soundcloud / vimeo players
	for i, iframe := range regexIframe.FindAllStringSubmatch(html, -1) {
		if strings.Contains(iframe[1], "soundcloud") {
			if strings.Contains(iframe[1], "track") {
				getAudio(iframe[1], lessonPath, i)
			}
		} else if strings.Contains(iframe[1], "vimeo") {
			getVideo(regexVimeoVideoLink.FindStringSubmatch(iframe[1])[1], lessonPath, i)
		}
	}
	fmt.Println(lesson)
}

func getAudio(audioLink, lessonPath string, i int) {
	secretToken := regexSecretToken.FindString(audioLink)
	trackNumber := regexTrackNumber.FindStringSubmatch(audioLink)[1]
	request, err := http.NewRequest(
		"GET",
		fmt.Sprintf(
			"https://api-widget.soundcloud.com/resolve?url=https://api.soundcloud.com/tracks/%s?%sformat=json&%s",
			trackNumber,
			secretToken,
			clientId,
		),
		nil,
	)
	if err != nil {
		panic(err)
	}
	response, err := client.Do(request)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}
	var resolve soundCloudResolve
	err = json.Unmarshal(body, &resolve)
	if err != nil {
		panic(err)
	}

	urlKurwa := resolve.Media.Transcodings[0].Url

	if strings.Contains(urlKurwa, "?") {
		urlKurwa += "&"
	} else {
		urlKurwa += "?"
	}

	request, err = http.NewRequest("GET", urlKurwa+clientId, nil)
	if err != nil {
		panic(err)
	}
	response, err = client.Do(request)
	if err != nil {
		panic(err)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}

	var playListUrl m3u8PlaylistUrl
	err = json.Unmarshal(body, &playListUrl)
	if err != nil {
		panic(err)
	}

	request, err = http.NewRequest("GET", playListUrl.Url, nil)
	if err != nil {
		panic(err)
	}
	response, err = client.Do(request)
	if err != nil {
		panic(err)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}
	var links []string
	for _, link := range strings.Split(string(body), "\n") {
		if link[0] != '#' {
			links = append(links, link)
		}
	}

	var audio []byte

	for _, link := range links {
		request, err = http.NewRequest("GET", link, nil)
		if err != nil {
			panic(err)
		}
		response, err = client.Do(request)
		if err != nil {
			panic(err)
		}
		body, err = io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		err = response.Body.Close()
		if err != nil {
			panic(err)
		}
		audio = append(audio, body...)
	}

	err = os.WriteFile(
		fmt.Sprintf("%ssoundcloudaudio%d.mp3", lessonPath, i),
		audio,
		0666,
	)
	if err != nil {
		panic(err)
	}
}

func getVideo(videoLink, lessonPath string, i int) {
	jebacCloudflare := 0
start:
	request, err := http.NewRequest("GET", videoLink, nil)
	if err != nil {
		panic(err)
	}
	request.Header.Set("User-Agent", "[[user agent]]")
	request.Header.Set("sec-ch-ua-platform", `[[sec ch ua platform]]`)
	response, err := client.Do(request)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}

	html := string(body)

	if regexJustAMoment.FindString(html) != "" || html == "error code: 524" {
		jebacCloudflare++
		if jebacCloudflare < 2 {
			time.Sleep(7 * time.Second)
			goto start
		}
		panic("kurwa: cloudflare")
	}

	avcLink := regexAvcUrl.FindStringSubmatch(html)[1]
	avcLink = strings.ReplaceAll(avcLink, `\u0026`, "&")
	request, err = http.NewRequest("GET", avcLink, nil)
	if err != nil {
		panic(err)
	}
	response, err = client.Do(request)
	if err != nil {
		panic(err)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		panic(err)
	}
	err = response.Body.Close()
	if err != nil {
		panic(err)
	}
	var data vimeoVideoData
	err = json.Unmarshal(body, &data)
	if err != nil {
		panic(err)
	}

	var bestVideo videoData
	bestBitRateVideo := videoData{
		Bitrate: 0,
	} // case for fucking random values of height and width of video
	for _, video := range data.Video {
		if video.Bitrate > bestBitRateVideo.Bitrate {
			bestBitRateVideo = video
		}
		if (video.Width == 1920 && video.Height == 1080) || (video.Width == 1080 && video.Height == 1920) { // || (video.Width == 1174 || video.Width == 1278 || video.Width == 1348 || video.Width == 1352 && video.Height == 720) || (video.Width == 1366 && video.Height == 690) {
			bestVideo = video
			break
		}
	}

	if bestVideo.Width == 0 {
		bestVideo = bestBitRateVideo
	}

	videoInit, err := base64.StdEncoding.DecodeString(bestVideo.InitSegment)
	if err != nil {
		panic(err)
	}

	bestAudio := audioData{
		Bitrate: 0,
	}
	for _, audio := range data.Audio {
		if audio.Bitrate > bestAudio.Bitrate {
			bestAudio = audio
		}
	}

	audioInit, err := base64.StdEncoding.DecodeString(bestAudio.InitSegment)
	if err != nil {
		panic(err)
	}

	parentDir := strings.Count(data.BaseUrl, "../")
	baseUrl := avcLink
	for range parentDir + 1 {
		baseUrl = path.Dir(baseUrl)
	}
	baseUrl = baseUrl + "/" + data.BaseUrl[3*parentDir:]

	err = syscall.Mkfifo(lessonPath+"video_pipe", 0666)
	if err != nil {
		panic(err)
	}
	err = syscall.Mkfifo(lessonPath+"audio_pipe", 0666)
	if err != nil {
		panic(err)
	}

	go func() {
		videoPipe, err := os.OpenFile(lessonPath+"video_pipe", os.O_WRONLY, 0666)
		if err != nil {
			panic(err)
		}
		defer videoPipe.Close()

		_, err = videoPipe.Write(videoInit)
		if err != nil {
			panic(err)
		}

		parentDirVideo := strings.Count(bestVideo.BaseUrl, "../")
		baseUrlVideo := baseUrl
		for range parentDirVideo + 1 {
			baseUrlVideo = path.Dir(baseUrlVideo)
		}
		linkBase := baseUrlVideo + "/" + bestVideo.BaseUrl[3*parentDirVideo:]
		linkBase = strings.ReplaceAll(linkBase, "https:/", "https://")

		for _, segment := range bestVideo.Segments {
			link := linkBase + segment.Url
			requestVideo, err := http.NewRequest("GET", link, nil)
			if err != nil {
				panic(err)
			}
			responseVideo, err := client.Do(requestVideo)
			if err != nil {
				panic(err)
			}
			bodyVideo, err := io.ReadAll(responseVideo.Body)
			if err != nil {
				panic(err)
			}
			err = responseVideo.Body.Close()
			if err != nil {
				panic(err)
			}

			_, err = videoPipe.Write(bodyVideo)
			if err != nil {
				panic(err)
			}
		}
	}()

	go func() {
		audioPipe, err := os.OpenFile(lessonPath+"audio_pipe", os.O_WRONLY, 0666)
		if err != nil {
			panic(err)
		}
		defer audioPipe.Close()

		_, err = audioPipe.Write(audioInit)
		if err != nil {
			panic(err)
		}

		parentDirAudio := strings.Count(bestAudio.BaseUrl, "../")
		baseUrlAudio := baseUrl
		for range parentDirAudio + 1 {
			baseUrlAudio = path.Dir(baseUrlAudio)
		}
		linkBase := baseUrlAudio + "/" + bestAudio.BaseUrl[3*parentDirAudio:]
		linkBase = strings.ReplaceAll(linkBase, "https:/", "https://")

		for _, segment := range bestAudio.Segments {
			link := linkBase + segment.Url
			requestAudio, err := http.NewRequest("GET", link, nil)
			if err != nil {
				panic(err)
			}
			responseAudio, err := client.Do(requestAudio)
			if err != nil {
				panic(err)
			}
			bodyAudio, err := io.ReadAll(responseAudio.Body)
			if err != nil {
				panic(err)
			}

			if len(bodyAudio) == 0 {
				fmt.Println(bestAudio.InitSegment)
				fmt.Println()
				fmt.Println(link)
				panic("co jest kurwa z tym aduio???")
			}

			err = responseAudio.Body.Close()
			if err != nil {
				panic(err)
			}

			_, err = audioPipe.Write(bodyAudio)
			if err != nil {
				panic(err)
			}
		}
	}()

	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-i", lessonPath+"video_pipe",
		"-i", lessonPath+"audio_pipe",
		"-c", "copy",
		fmt.Sprintf("%soutput%d.mp4", lessonPath, i),
	)
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		fmt.Println(lessonPath)
		os.WriteFile(lessonPath+"audio.mp4", audioInit, 0666)
		os.WriteFile(lessonPath+"video.mp4", videoInit, 0666)
		panic(err)
	}

	os.Remove(lessonPath + "video_pipe")
	os.Remove(lessonPath + "audio_pipe")
}
