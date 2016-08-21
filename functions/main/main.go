package main

import (
	"encoding/json"
//	"fmt"
	"github.com/apex/go-apex"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/awslabs/aws-sdk-go/service/ec2"
	"github.com/joho/godotenv"
	"os"
    "time"
)

var context *apex.Context

// 環境にあったクレデンシャルを取得する
func AwsConfig() *aws.Config {
	godotenv.Load()
	//fmt.Println(context)
    
	cr := credentials.NewEnvCredentials()
    
    if context != nil {
		return &aws.Config{
			Region:      aws.String(os.Getenv("REGION")),
		    Credentials: cr,
		}    	
    } else {
	    return &aws.Config{
	        Region:      aws.String(os.Getenv("REGION")),
	        Credentials: credentials.NewSharedCredentials("", os.Getenv("credential-profile")),
	    }
    }
    return nil
}

// SNSにメッセージをPushする
func sendMessageBySns(msg string,subject string) error {
	
	sess, err := session.NewSession(AwsConfig())

    if err != nil {
        return err
    }
	
	svc := sns.New(sess)
	
	params := &sns.PublishInput{
		Message: aws.String(msg),
		Subject: aws.String(subject),
		TopicArn: aws.String(os.Getenv("SNS_TOPIC")),
	}
	_, err = svc.Publish(params) 
	
	if err != nil {
	    return err
	}
	
	return nil
}

func main() {
	apex.HandleFunc(func(event json.RawMessage, ctx *apex.Context) (interface{}, error) {
		
		context = ctx
		var targetInstanceId string
    	var attachedEbs []string
		var volumeId string
		var msg string
		var subject string

		// 環境変数を読み込み
		err := godotenv.Load()
		if err != nil {
	        return nil, err
		}
		
		sess, err := session.NewSession(AwsConfig())

	    if err != nil {
	        return nil, err
	    }

		// .env内のインスタンスにマウントされているEBSのIDを取得する
		svc := ec2.New(sess)

		targetInstanceId = os.Getenv("INSTANCE_ID")
	    params := &ec2.DescribeInstanceAttributeInput{
	        Attribute:  aws.String("blockDeviceMapping"),
	        InstanceId: aws.String(targetInstanceId),
	    }
	
	    resp, err := svc.DescribeInstanceAttribute(params)
			
	    if err != nil {
	        return nil, err
	    }
	    
	    // 取得したEBSのIDをスライスに格納する
	    for i := 0; i < len(resp.BlockDeviceMappings); i++{
			volumeId = *resp.BlockDeviceMappings[i].Ebs.VolumeId
			attachedEbs = append(attachedEbs, volumeId)
		}

		for i := 0; i < len(attachedEbs); i++ {
			completeFlag := false

			// EBSから生成されたスナップショットを取得
			params := &ec2.DescribeSnapshotsInput{
				Filters: []*ec2.Filter{
					{
						Name: aws.String("volume-id"),
						Values: []*string{
							aws.String(attachedEbs[i]),
						},
					},
				},
			}
			resp, err := svc.DescribeSnapshots(params)
		    if err != nil {
		        return nil, err
		    }

			// スナップショットのリストから、作成日付とステータスを取得
			for j := 0; j < len(resp.Snapshots); j++ {
				now := time.Now().YearDay()
				startTime := resp.Snapshots[j].StartTime.YearDay()
				state := resp.Snapshots[j].State
				
				// もし、スナップショットを作った日と今日が同じ日で、かつステータスがcomplteであれば、フラグを立てる
				if now == startTime && *state == "completed" {
					completeFlag = true
				}
			}

			// フラグがtrueなら、OKのメッセージを。フラグがfalseならerrorのメッセージを。
			if completeFlag == true {
				msg = "Creating the snapshot of " + os.Getenv("INSTANCE_ID") + " was successed."
				subject = "【SUCCESS】aws snapshot backup"
			} else {
				msg = "Creating the snapshot of " + os.Getenv("INSTANCE_ID") + " was failed."
				subject = "【ERROR】aws snapshot backup"
			}

			// 上記で作ったメッセージを使って、SNSにpublish
			err = sendMessageBySns(msg,subject)	
			if err != nil {
				return nil, err
			}

		}
		return nil, nil
	})
}
