package object

type Note struct {
	Context      []interface{} `json:"@context"`
	Id           string        `json:"id"`
	Type         string        `json:"type"`
	AttributedTo string        `json:"attributedTo"`
	Content      string        `json:"content"`
}
