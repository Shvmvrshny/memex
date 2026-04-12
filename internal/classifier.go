package memex

import (
	"regexp"
	"strings"
)

// typeMarkers maps memory type → list of keyword/phrase patterns.
// Ported from mempalace general_extractor.py and extended to 9 types.
var typeMarkers = map[string][]string{
	"decision": {
		`let'?s (use|go with|try|pick|choose|switch to)`,
		`we (decided|chose|went with|settled on|picked)`,
		`rather than`,
		`architecture`,
		`approach`,
		`strategy`,
		`configure`,
		`trade-?off`,
	},
	"preference": {
		`i prefer`,
		`always use`,
		`never use`,
		`don'?t (ever )?(use|do|mock|stub)`,
		`i like (to|when|how)`,
		`i hate (when|how)`,
		`my (rule|preference|style|convention) is`,
		`we (always|never)`,
		`snake_?case`,
		`camel_?case`,
	},
	"event": {
		`session on`,
		`we met`,
		`sprint`,
		`last week`,
		`deployed`,
		`shipped`,
		`launched`,
		`milestone`,
		`standup`,
		`released`,
	},
	"discovery": {
		`it works`,
		`it worked`,
		`got it working`,
		`figured (it )?out`,
		`turns out`,
		`the trick (is|was)`,
		`realized`,
		`breakthrough`,
		`finally`,
		`now i (understand|see|get it)`,
		`the key (is|was)`,
	},
	"advice": {
		`you should`,
		`recommend`,
		`best practice`,
		`the answer (is|was)`,
		`suggestion`,
		`consider using`,
		`try using`,
		`better to`,
	},
	"problem": {
		`\b(bug|error|crash|fail|broke|broken|issue|problem)\b`,
		`doesn'?t work`,
		`not working`,
		`keeps? (failing|crashing|breaking)`,
		`root cause`,
		`the (problem|issue|bug) (is|was)`,
		`workaround`,
	},
	"context": {
		`works on`,
		`responsible for`,
		`reports to`,
		`\bteam\b`,
		`\bowns\b`,
		`based in`,
		`member of`,
		`\bleads\b`,
	},
	"procedure": {
		`steps to`,
		`how to`,
		`\bworkflow\b`,
		`always run`,
		`first run`,
		`then run`,
		`\bpipeline\b`,
		`\bprocess\b`,
	},
	"rationale": {
		`the reason we`,
		`chose over`,
		`we rejected`,
		`instead of`,
		`because we need`,
		`pros and cons`,
	},
}

// resolutionMarkers detect that a problem has been resolved (→ reclassify as discovery).
var resolutionMarkers = []*regexp.Regexp{
	regexp.MustCompile(`\bfixed\b`),
	regexp.MustCompile(`\bsolved\b`),
	regexp.MustCompile(`\bresolved\b`),
	regexp.MustCompile(`\bgot it working\b`),
	regexp.MustCompile(`\bit works\b`),
	regexp.MustCompile(`\bthe fix (is|was)\b`),
	regexp.MustCompile(`\bfigured (it )?out\b`),
}

// positiveWords for sentiment disambiguation.
var positiveWords = map[string]bool{
	"fixed": true, "solved": true, "works": true, "working": true,
	"breakthrough": true, "success": true, "nailed": true, "figured": true,
}

// negativeWords for sentiment disambiguation.
var negativeWords = map[string]bool{
	"bug": true, "error": true, "crash": true, "fail": true, "failed": true,
	"broken": true, "broke": true, "issue": true, "problem": true, "stuck": true,
}

// compiledMarkers caches compiled regexp for each type.
type compiledMarkerSet map[string][]*regexp.Regexp

// Classifier classifies a text string into one of 9 memory types.
type Classifier struct {
	compiled compiledMarkerSet
}

// NewClassifier compiles all marker patterns once and returns a ready Classifier.
func NewClassifier() *Classifier {
	compiled := make(compiledMarkerSet, len(typeMarkers))
	for memType, patterns := range typeMarkers {
		for _, p := range patterns {
			compiled[memType] = append(compiled[memType], regexp.MustCompile(`(?i)`+p))
		}
	}
	return &Classifier{compiled: compiled}
}

// Classify returns the best-match memory type and a confidence score (0.0–1.0).
// Returns ("", 0) if no markers match or confidence < 0.3.
func (c *Classifier) Classify(text string) (string, float64) {
	lower := strings.ToLower(text)
	scores := make(map[string]float64)

	for memType, patterns := range c.compiled {
		for _, re := range patterns {
			if re.MatchString(lower) {
				scores[memType] += 1.0
			}
		}
	}
	if len(scores) == 0 {
		return "", 0
	}

	// Length bonus
	var bonus float64
	if len(text) > 500 {
		bonus = 2
	} else if len(text) > 200 {
		bonus = 1
	}

	bestType := ""
	bestScore := 0.0
	for t, s := range scores {
		if s > bestScore {
			bestScore = s
			bestType = t
		}
	}
	bestScore += bonus

	// Disambiguation: problem + resolution → discovery
	if bestType == "problem" {
		for _, re := range resolutionMarkers {
			if re.MatchString(lower) {
				bestType = "discovery"
				break
			}
		}
		// Problem + positive sentiment → discovery
		if bestType == "problem" && c.sentiment(lower) == "positive" {
			if scores["discovery"] > 0 {
				bestType = "discovery"
			}
		}
	}

	confidence := bestScore / 3.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.3 {
		return "", confidence
	}
	return bestType, confidence
}

func (c *Classifier) sentiment(text string) string {
	words := strings.Fields(text)
	pos, neg := 0, 0
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:'\"")
		if positiveWords[w] {
			pos++
		}
		if negativeWords[w] {
			neg++
		}
	}
	if pos > neg {
		return "positive"
	}
	if neg > pos {
		return "negative"
	}
	return "neutral"
}
