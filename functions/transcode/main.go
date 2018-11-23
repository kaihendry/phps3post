package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/apex/log"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3post "github.com/kaihendry/s3post/struct"
)

type S3PostSNS struct {
	Records []struct {
		EventSource          string `json:"EventSource"`
		EventVersion         string `json:"EventVersion"`
		EventSubscriptionArn string `json:"EventSubscriptionArn"`
		Sns                  struct {
			Type              string      `json:"Type"`
			MessageID         string      `json:"MessageId"`
			TopicArn          string      `json:"TopicArn"`
			Subject           interface{} `json:"Subject"`
			Message           string      `json:"Message"`
			Timestamp         time.Time   `json:"Timestamp"`
			SignatureVersion  string      `json:"SignatureVersion"`
			Signature         string      `json:"Signature"`
			SigningCertURL    string      `json:"SigningCertUrl"`
			UnsubscribeURL    string      `json:"UnsubscribeUrl"`
			MessageAttributes struct {
			} `json:"MessageAttributes"`
		} `json:"Sns"`
	} `json:"Records"`
}

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, evt S3PostSNS) (string, error) {

	var uploadObject s3post.S3upload

	err := json.Unmarshal([]byte(evt.Records[0].Sns.Message), &uploadObject)
	if err != nil {
		return "", err
	}

	suffix := filepath.Ext(uploadObject.Key)
	log.WithFields(log.Fields{
		"suffix":       suffix,
		"uploadObject": uploadObject,
	}).Info("switch")

	switch suffix {
	case ".txt":
		log.Info("txt file")
		src, err := get(uploadObject)
		if err != nil {
			log.WithError(err).Error("failed to retrieve src file to lambda")
			return "", err
		}
		dst := "/tmp/newhello"
		err = addHello(src, dst)
		if err != nil {
			log.WithError(err).Error("failed to add hello")
			return "", err
		}
		err = put(dst, uploadObject)
		if err != nil {
			log.WithError(err).Error("failed to put")
			return "", err
		}

	default:
		log.Warn("unrecognized suffix")
	}

	return "", nil
}

func addHello(src string, dst string) (err error) {
	content, err := ioutil.ReadFile(src)
	if err != nil {
		log.WithError(err).Error("error reading")
		return err
	}

	var out []byte
	path, err := exec.LookPath("./hello/hello")
	if err != nil {
		log.WithError(err).Error("no hello binary found")
		return err
	}
	out, err = exec.Command(path).CombinedOutput()
	if err != nil {
		log.WithError(err).Errorf("hello failed: %s", out)
		return err
	}

	err = ioutil.WriteFile(dst, append(content, out...), 0644)
	return err
}

func put(src string, dst s3post.S3upload) (err error) {
	log.Infof("Putting %s on %v", src, dst)
	cfg, err := external.LoadDefaultAWSConfig(external.WithSharedConfigProfile("mine"))
	if err != nil {
		return err
	}

	svc := s3.New(cfg)

	f, err := os.Open(src)
	if err != nil {
		log.WithError(err).Fatal("unable to open src")
		return err
	}
	defer f.Close()

	putparams := &s3.PutObjectInput{
		Bucket:      aws.String(dst.Bucket),
		Body:        aws.ReadSeekCloser(f),
		Key:         aws.String(dst.Key),
		ACL:         s3.ObjectCannedACLPublicRead,
		ContentType: aws.String(fmt.Sprintf("%s; charset=UTF-8", dst.ContentType)),
	}

	req := svc.PutObjectRequest(putparams)
	_, err = req.Send()
	if err != nil {
		log.WithError(err).Fatal("failed to upload to s3")
		return err
	}

	return nil
}

func get(src s3post.S3upload) (dst string, err error) {
	cfg, err := external.LoadDefaultAWSConfig(external.WithSharedConfigProfile("mine"))
	if err != nil {
		return "", err
	}

	svc := s3.New(cfg)

	input := &s3.GetObjectInput{
		Bucket: aws.String(src.Bucket),
		Key:    aws.String(src.Key),
	}

	req := svc.GetObjectRequest(input)
	res, err := req.Send()
	if err != nil {
		log.WithError(err).Fatal("failed to get file")
		return "", err
	}

	dst = "/tmp/transcoded"
	outFile, err := os.Create(dst)
	if err != nil {
		log.WithError(err).Fatal("failed to create output file")
		return "", err
	}

	defer outFile.Close()
	_, err = io.Copy(outFile, res.Body)

	return dst, err
}
