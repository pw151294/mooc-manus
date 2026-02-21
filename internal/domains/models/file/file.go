package file

import "github.com/openai/openai-go"

type File struct {
	ID        string
	FileName  string
	FilePath  string
	Key       string
	Extension string
	MimeType  string
	Size      int
}

func Convert2UserMessage(query string, files []File) openai.ChatCompletionMessageParamUnion {
	userMessage := openai.UserMessage(query)
	if len(files) > 0 {
		contents := make([]openai.ChatCompletionContentPartUnionParam, 0, len(files))
		for _, file := range files {
			fileFileParam := openai.ChatCompletionContentPartFileFileParam{}
			fileFileParam.FileID = openai.String(file.ID)
			fileFileParam.Filename = openai.String(file.FileName)
			fileFileParam.FileData = openai.String(file.FilePath)

			fileParam := &openai.ChatCompletionContentPartFileParam{}
			fileParam.File = fileFileParam

			content := openai.ChatCompletionContentPartUnionParam{}
			content.OfFile = fileParam
			contents = append(contents, content)
		}
		userMessage.OfUser.Content.OfArrayOfContentParts = contents
	}
	return userMessage
}

func ConvertMessage2QueryAndAttachments(message openai.ChatCompletionMessageParamUnion) (string, []string) {
	var (
		query       string
		attachments []string
	)

	if message.OfUser == nil {
		return "", nil
	}
	if s := message.OfUser.Content.OfString.String(); s != "" {
		query = s
	}
	contents := message.OfUser.Content.OfArrayOfContentParts
	for _, part := range contents {
		if part.OfText != nil {
			if t := part.OfText.Text; t != "" {
				query += t
			}
		}
		if part.OfFile != nil {
			if fd := part.OfFile.File.FileData.String(); fd != "" {
				attachments = append(attachments, fd)
			}
		}
	}

	return query, attachments
}
