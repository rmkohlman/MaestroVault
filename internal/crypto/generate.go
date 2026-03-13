package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const (
	lowercaseChars = "abcdefghijklmnopqrstuvwxyz"
	uppercaseChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitChars     = "0123456789"
	symbolChars    = "!@#$%^&*()-_=+[]{}|;:,.<>?"
)

// GenerateOpts configures password generation.
type GenerateOpts struct {
	Length    int
	Uppercase bool
	Lowercase bool
	Digits    bool
	Symbols   bool
}

// DefaultGenerateOpts returns sensible defaults for password generation.
func DefaultGenerateOpts() GenerateOpts {
	return GenerateOpts{
		Length:    32,
		Uppercase: true,
		Lowercase: true,
		Digits:    true,
		Symbols:   true,
	}
}

// GeneratePassword generates a cryptographically random password using the
// specified character sets. At least one character from each enabled set is
// guaranteed to appear in the output.
func GeneratePassword(opts GenerateOpts) (string, error) {
	if opts.Length < 1 {
		return "", fmt.Errorf("password length must be at least 1")
	}

	var charset string
	var required []string

	if opts.Lowercase {
		charset += lowercaseChars
		required = append(required, lowercaseChars)
	}
	if opts.Uppercase {
		charset += uppercaseChars
		required = append(required, uppercaseChars)
	}
	if opts.Digits {
		charset += digitChars
		required = append(required, digitChars)
	}
	if opts.Symbols {
		charset += symbolChars
		required = append(required, symbolChars)
	}

	if charset == "" {
		return "", fmt.Errorf("at least one character set must be enabled")
	}

	if opts.Length < len(required) {
		return "", fmt.Errorf("password length %d is too short to include all required character sets (%d)", opts.Length, len(required))
	}

	// Build the password ensuring at least one char from each required set.
	password := make([]byte, opts.Length)

	// First, place one random character from each required set.
	for i, reqSet := range required {
		ch, err := randomChar(reqSet)
		if err != nil {
			return "", err
		}
		password[i] = ch
	}

	// Fill remaining positions from the full charset.
	for i := len(required); i < opts.Length; i++ {
		ch, err := randomChar(charset)
		if err != nil {
			return "", err
		}
		password[i] = ch
	}

	// Fisher-Yates shuffle to avoid predictable positions for required chars.
	if err := shuffleBytes(password); err != nil {
		return "", err
	}

	return string(password), nil
}

// GeneratePassphrase generates a passphrase of the given word count using a
// built-in word list. Words are separated by the specified delimiter.
func GeneratePassphrase(wordCount int, delimiter string) (string, error) {
	if wordCount < 1 {
		return "", fmt.Errorf("word count must be at least 1")
	}

	words := make([]string, wordCount)
	for i := 0; i < wordCount; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(wordList))))
		if err != nil {
			return "", fmt.Errorf("generating random index: %w", err)
		}
		words[i] = wordList[idx.Int64()]
	}

	return strings.Join(words, delimiter), nil
}

// GenerateToken generates a cryptographically random hex token of the
// specified byte length (output will be 2x bytes in hex characters).
func GenerateToken(byteLength int) (string, error) {
	if byteLength < 1 {
		return "", fmt.Errorf("byte length must be at least 1")
	}
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

func randomChar(charset string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	if err != nil {
		return 0, fmt.Errorf("generating random char: %w", err)
	}
	return charset[n.Int64()], nil
}

func shuffleBytes(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("shuffling: %w", err)
		}
		b[i], b[j.Int64()] = b[j.Int64()], b[i]
	}
	return nil
}

// wordList is a curated diceware-style word list for passphrase generation.
// Short, common, easy-to-type words.
var wordList = []string{
	"able", "acid", "aged", "also", "area", "army", "away", "baby", "back", "ball",
	"band", "bank", "base", "bath", "bear", "beat", "been", "beer", "bell", "belt",
	"best", "bill", "bird", "bite", "blow", "blue", "boat", "body", "bomb", "bond",
	"bone", "book", "boot", "born", "boss", "both", "bowl", "bulk", "burn", "bush",
	"busy", "call", "calm", "came", "camp", "card", "care", "case", "cash", "cast",
	"cell", "chat", "chip", "city", "claim", "clay", "clip", "club", "clue", "coat",
	"code", "cold", "come", "cook", "cool", "cope", "copy", "core", "cost", "crew",
	"crop", "cure", "cute", "dale", "damn", "dare", "dark", "data", "date", "dawn",
	"dead", "deal", "dear", "debt", "deep", "deer", "demo", "deny", "desk", "dial",
	"dice", "diet", "dirt", "dish", "disk", "dock", "does", "done", "door", "dose",
	"down", "draw", "drew", "drop", "drug", "drum", "dual", "duke", "dull", "dump",
	"dust", "duty", "each", "earn", "ease", "east", "easy", "edge", "else", "epic",
	"even", "ever", "evil", "exam", "exec", "exit", "face", "fact", "fail", "fair",
	"fall", "fame", "farm", "fast", "fate", "fear", "feed", "feel", "feet", "fell",
	"felt", "file", "fill", "film", "find", "fine", "fire", "firm", "fish", "five",
	"flag", "flat", "flew", "flip", "flow", "fold", "folk", "food", "foot", "ford",
	"fore", "fork", "form", "fort", "foul", "four", "free", "from", "fuel", "full",
	"fund", "fury", "fuse", "gain", "game", "gang", "gate", "gave", "gaze", "gear",
	"gene", "gift", "girl", "give", "glad", "glow", "glue", "goal", "goes", "gold",
	"golf", "gone", "good", "grab", "gray", "grew", "grid", "grip", "grow", "gulf",
	"guru", "hack", "hair", "half", "hall", "halt", "hand", "hang", "hard", "harm",
	"hash", "hate", "have", "head", "heap", "heat", "held", "hell", "help", "here",
	"hero", "hide", "high", "hike", "hill", "hint", "hire", "hold", "hole", "holy",
	"home", "hood", "hook", "hope", "horn", "host", "hour", "huge", "hung", "hunt",
	"hurt", "idea", "inch", "into", "iron", "item", "jack", "jail", "jazz", "jean",
	"jobs", "join", "joke", "jump", "jury", "just", "keen", "keep", "kent", "kept",
	"kick", "kill", "kind", "king", "knee", "knew", "knit", "knot", "know", "lack",
	"laid", "lake", "lamp", "land", "lane", "last", "late", "lead", "leaf", "lean",
	"left", "lend", "lens", "less", "lick", "life", "lift", "like", "limb", "lime",
	"limp", "line", "link", "lion", "list", "live", "load", "loan", "lock", "logo",
	"long", "look", "lord", "lose", "loss", "lost", "love", "luck", "lump", "lung",
	"made", "mail", "main", "make", "male", "mall", "many", "mark", "mask", "mass",
	"mate", "maze", "meal", "mean", "meat", "meet", "melt", "memo", "menu", "mere",
	"mesh", "mess", "mild", "mile", "milk", "mill", "mind", "mine", "mint", "miss",
	"mode", "mood", "moon", "more", "most", "move", "much", "must", "myth", "nail",
	"name", "navy", "near", "neat", "neck", "need", "nest", "news", "next", "nice",
	"nine", "node", "none", "norm", "nose", "note", "noun", "odds", "okay", "once",
	"only", "onto", "open", "oral", "oven", "over", "pace", "pack", "page", "paid",
	"pain", "pair", "pale", "palm", "pane", "para", "park", "part", "pass", "past",
	"path", "peak", "peer", "pick", "pile", "pine", "pink", "pipe", "plan", "play",
	"plea", "plot", "plug", "plus", "poem", "poet", "pole", "poll", "pond", "pool",
	"poor", "pope", "pork", "port", "pose", "post", "pour", "pray", "prop", "pull",
	"pump", "pure", "push", "quit", "quiz", "race", "rack", "rage", "raid", "rail",
	"rain", "rank", "rare", "rate", "read", "real", "rear", "rely", "rent", "rest",
	"rice", "rich", "ride", "ring", "rise", "risk", "road", "rock", "rode", "role",
	"roll", "roof", "room", "root", "rope", "rose", "ruin", "rule", "rush", "ruth",
	"safe", "said", "sake", "sale", "salt", "same", "sand", "sang", "save", "seal",
	"seat", "seed", "seek", "seem", "seen", "self", "sell", "send", "sent", "sept",
	"shed", "shin", "ship", "shop", "shot", "show", "shut", "sick", "side", "sign",
	"silk", "sing", "sink", "site", "size", "skin", "slam", "slid", "slim", "slip",
	"slot", "slow", "snap", "snow", "sock", "soft", "soil", "sold", "sole", "some",
	"song", "soon", "sort", "soul", "span", "spin", "spot", "star", "stay", "stem",
	"step", "stir", "stop", "such", "suit", "sung", "sure", "swim", "tail", "take",
	"tale", "talk", "tall", "tank", "tape", "task", "taxi", "team", "tear", "tell",
	"temp", "tend", "tent", "term", "test", "text", "than", "that", "them", "then",
	"they", "thin", "this", "thus", "tick", "tide", "tidy", "tied", "tier", "tile",
	"till", "time", "tiny", "tire", "toad", "toil", "told", "toll", "tone", "took",
	"tool", "tops", "tore", "torn", "tour", "town", "trap", "tray", "tree", "trim",
	"trio", "trip", "true", "tube", "tuck", "tune", "turn", "twin", "type", "ugly",
	"unit", "unto", "upon", "urge", "used", "user", "uses", "vale", "vary", "vast",
	"verb", "very", "vice", "view", "vine", "visa", "void", "volt", "vote", "wade",
	"wage", "wait", "wake", "walk", "wall", "want", "ward", "warm", "warn", "wash",
	"vast", "wave", "ways", "weak", "wear", "weed", "week", "well", "went", "were",
	"west", "what", "when", "whom", "wide", "wife", "wild", "will", "wind", "wine",
	"wing", "wire", "wise", "wish", "with", "woke", "wolf", "wood", "word", "wore",
	"work", "worm", "worn", "wrap", "yard", "yarn", "yeah", "year", "yell", "zero",
	"zone", "zoom",
}
