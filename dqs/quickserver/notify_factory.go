package quickserver

import (
	//	"bytes"

	"bufio"
	"dqs/util"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	log "github.com/cihub/seelog"
	"github.com/jpoehls/gophermail"
	"io/ioutil"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MmsServiceCode struct {
	XMLName xml.Name `xml:"string"`
	Code    int64    `xml:"string"`
}

func CheckAndSendNotify(report Report) {
	//发送条件判断
	needAudit := GlobalConfig.ReportCfg.AuditBeforeSend
	directSendLevel := GlobalConfig.ReportCfg.MinDirectSendLevel

	//不需要审核,则直接发送
	if needAudit == false {
		log.Info("无需审核,直接发送速报.")
		PrepareMms(report)
	} else {
		//如果级别大于审核级别,则直接发送
		if report.Event.MaxLevel >= directSendLevel {
			log.Infof("事件最高烈度%d 高于 设定级别%d, 无需审核,直接发送.", report.Event.MaxLevel, directSendLevel)
			PrepareMms(report)
		} else {
			//如果已经通过审核.则发送.
			if report.Verify {
				log.Info("速报已通过审核,准备发送.")
				PrepareMms(report)
			} else {
				log.Info("速报发送需要审核,该速报还未通过审核,不会直接发送.")
			}
		}
	}

}

//准备发送彩信
func PrepareMms(report Report) {

	sended := false

	num, mmsUsers, mailerList := getValidReceivers()

	//存在彩信服务才发送
	mmscfg := SystemConfigs.MmsCfg
	if mmscfg.MmsEnable == true {
		if mmscfg.ServiceUrl != "" && mmscfg.UserNo != "" && mmscfg.Password != "" {

			//有多于1个的接收人才发送
			if num > 0 {
				log.Infof("该速报将会向%d个用户发送彩信通知.", num)
				sendMms(report, mmsUsers)
				sended = true
			} else {
				log.Info("无接收人,停止发送彩信.")
			}
		} else {
			log.Warnf("彩信服务配置信息不完整,停止发送彩信.")
		}
	} else {
		log.Infof("系统设置-彩信发送停用.")
	}
	//检查邮件服务,发送邮件通知

	if len(mailerList) > 0 {
		sendMail(report, mailerList)
		sended = true
	} else {
		log.Info("无邮件接收者,不会发送邮件速报.")
	}

	//更新速报发送状态
	if sended {
		updateReportSendStatus(&report)
	}
}

//发送彩信
func sendMms(report Report, users string) {
	//彩信服务账户信息
	sn := SystemConfigs.MmsCfg.UserNo
	password := SystemConfigs.MmsCfg.Password
	pwd := util.GetMd5Hex(sn + password)

	mmsServiceUrl := SystemConfigs.MmsCfg.ServiceUrl
	//彩信图片地址
	dir := GlobalConfig.FileConfig.ReportFileDir

	title := fmt.Sprintf("Quake %s", report.Summary.EventTime)
	mmsText := fmt.Sprintf("事件时间:%s", report.Summary.EventTime)
	mmsText += fmt.Sprintf("\n报警数量:%d", report.Summary.AlarmCount)
	mmsText += fmt.Sprintf("\n报警统计:%s", report.Summary.Brief)
	if report.Event.IsConfirm {
		mmsText += fmt.Sprintf("\n地震数据:%s", report.Summary.QuakeInfo)
	}

	//编码转换
	gbtxt, errt := util.UTF8ToGBK(mmsText)
	if errt != nil {
		mmsText := fmt.Sprintf("Time:%s", report.Summary.EventTime)
		mmsText += fmt.Sprintf("\nAlarmS:%d", report.Summary.AlarmCount)
		mmsText += fmt.Sprintf("\nBrief:%s", report.Summary.Brief)
		if report.Event.IsConfirm {
			mmsText += fmt.Sprintf("\nEarthQuake:%s", report.Summary.QuakeInfo)
		}
		gbtxt = []byte(mmsText)
	}

	//发送内容
	ct := ""
	ct += "1_1.txt," + base64.StdEncoding.EncodeToString(gbtxt) + ";"
	ct += "1_2.jpg,"
	fc, err := ioutil.ReadFile(filepath.Join(dir, report.ImageFile))
	if err == nil {
		ct += base64.StdEncoding.EncodeToString(fc)
	}

	v := url.Values{}
	v.Add("sn", sn)
	v.Add("pwd", strings.ToUpper(pwd))
	v.Add("title", title)
	v.Add("mobile", users)
	v.Add("content", ct)
	v.Add("stime", "")

	//发送
	r, err := http.PostForm(mmsServiceUrl, v)

	defer r.Body.Close()

	if err != nil {
		log.Warnf("调用彩信接口出错:%s\n", err.Error())
		return
	}
	response, err2 := ioutil.ReadAll(r.Body)
	if err2 != nil {
		log.Warnf("读取彩信服务返回数据出错:%s\n", err2.Error())
		return
	}
	backcode := new(MmsServiceCode)
	err = xml.Unmarshal(response, backcode)
	if err == nil {
		if backcode.Code < 0 || backcode.Code == 1 {
			log.Errorf("彩信发送失败.code=%d", backcode.Code)
		} else {
			log.Infof("彩信发送成功")
		}
	} else {
		log.Warnf("无法解析彩信返回信息:%s", err.Error())
		log.Warnf("backcode=%s", response)
	}

}

//获取系统内可接收彩信的账号
func getValidReceivers() (mmscount int, mmsusers string, mailusers []string) {
	mmsreceivers := ""
	mmslist := []string{}
	mailerList := []string{}

	users, err := server.dataManager.GetValidUsers()
	if err != nil {
		log.Errorf("获取获取系统有效用户信息失败.%s", err.Error())
	}

	for _, v := range users {
		if v.ReportSet.ReportPhone && strings.TrimSpace(v.Mobile) != "" {
			mmslist = append(mmslist, v.Mobile)
		}
		if v.ReportSet.ReportMail && strings.TrimSpace(v.Email) != "" {
			mailerList = append(mailerList, v.Email)
		}
	}

	ucount := len(mmslist)
	for k, v := range mmslist {
		mmsreceivers += fmt.Sprintf("%s", v)
		if k < (ucount - 1) {
			mmsreceivers += ","
		}
	}

	return ucount, mmsreceivers, mailerList
}

//发送邮件
func sendMail(report Report, toUsers []string) {
	mailcfg := SystemConfigs.MailCfg
	host := mailcfg.MailHost
	port := mailcfg.MailPort
	addr := mailcfg.MailAddr
	auth := mailcfg.NeedAuth
	user := mailcfg.MailUser
	pwd := mailcfg.MailPassword

	dir := GlobalConfig.FileConfig.ReportFileDir

	if host != "" && port != "" && user != "" && pwd != "" {

		subject := fmt.Sprintf("Quake %s", report.Summary.EventTime)
		mmsText := fmt.Sprintf("<p>事件时间:%s</p>", report.Summary.EventTime)
		mmsText += fmt.Sprintf("<p>报警数量:%d</p>", report.Summary.AlarmCount)
		mmsText += fmt.Sprintf("<p>报警统计:%s</p>", report.Summary.Brief)
		if report.Event.IsConfirm {
			mmsText += fmt.Sprintf("<p>地震数据:%s</p>", report.Summary.QuakeInfo)
		}
		//构造邮件信息
		m := &gophermail.Message{}
		m.From = addr
		m.To = toUsers
		m.Subject = subject
		m.HTMLBody = mmsText
		m.Headers = mail.Header{}
		m.Headers["Date"] = []string{time.Now().UTC().Format(time.RFC822)}

		atta := gophermail.Attachment{}
		atta.Name = report.ImageFile
		fi, err := os.Open(filepath.Join(dir, report.ImageFile))
		if err != nil {
			log.Warnf("邮件图片读取失败:%s", err.Error())
		}
		atta.Data = bufio.NewReader(fi)
		defer fi.Close()
		m.Attachments = append(m.Attachments, atta)

		err = util.SendMulityMail(host, port, auth, user, pwd, m)
		if err != nil {
			log.Warnf("邮件速报发送失败:%s", err.Error())
		} else {
			log.Info("邮件速报发送成功")
		}
	}

}

//更新发送状态
func updateReportSendStatus(report *Report) {
	err := server.dataManager.UpdateReportSendStatus(report)
	if err != nil {
		log.Warnf("更新速报发送状态失败:%s", report.ReportId)
	}
}
