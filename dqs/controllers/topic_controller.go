package controllers

import (
	"dqs/dao"
	"dqs/models"

	log "github.com/cihub/seelog"
	"time"
)

type TopicController struct {
	BaseController
}

func (this *TopicController) TopicView() {
	this.Data["title"] = "专题图"
	this.Data["author"] = "wangzhigang"

	this.CheckUser()

	//当前时间
	now := time.Now()
	bTime := now.Add(time.Minute * -20)
	eTime := now.Add(time.Minute * 20)

	beginTime := bTime.Format(CommonTimeLayout)
	endTime := eTime.Format(CommonTimeLayout)

	this.Data["BeginTime"] = beginTime
	this.Data["EndTime"] = endTime

	allDevices, err := dao.GetAllDevices()
	if err != nil {
		log.Warnf("查询所有设备信息失败:%s", err.Error())
	}
	this.Data["allDevices"] = allDevices

	this.Data["GoogleMap"] = !SystemConfigs.DisableGoogleMap
	this.Data["mapExtend"] = SystemConfigs.GisImageCfg.BBOX

	this.Data["gisServiceUrl"] = SystemConfigs.GisServiceUrl
	this.Data["gisServiceParams"] = SystemConfigs.GisServiceParams
	this.Data["gisBasicLayer"] = SystemConfigs.GisLayerBasic
	this.Data["gisChinaLayer"] = SystemConfigs.GisLayerChina

	this.TplNames = "topic.html"
	this.Render()
}

//查询关联数据
func (this *TopicController) TopicData() {
	beginTime := this.GetString("beginTime")
	endTime := this.GetString("endTime")
	sensors := this.GetStrings("Sensors")

	var eventSignal models.EventSignal = models.EventSignal{}
	//查找报警数据
	alarms, err := dao.GetTopicData(beginTime, endTime, sensors)
	if err != nil {
		log.Warnf("查找等值线的报警数据时出错:%s", err.Error())
	}

	//是否加入网格化虚拟站点
	dataArray := NetGridCompute(alarms, eventSignal)

	data := make(map[string]interface{})
	//传递的数据值
	DataArrayStr := ""
	DataArrayStrPGA := ""
	DataArrayStrSI := ""
	var lastlng, lastlat float32

	for k, v := range dataArray {
		if k < len(dataArray)-1 {
			DataArrayStr += v.String() + ","
			DataArrayStrPGA += v.StringPGA() + ","
			DataArrayStrSI += v.StringSI() + ","
		} else {
			DataArrayStr += v.String()
			DataArrayStrPGA += v.StringPGA()
			DataArrayStrSI += v.StringSI()
		}
		//设置地图中心点位置
		if k == 0 {
			lastlng = v.Longitude
			lastlat = v.Latitude
		}
	}
	//添加系统参数
	data["dataArray"] = DataArrayStr
	data["dataArrayPGA"] = DataArrayStrPGA
	data["dataArraySI"] = DataArrayStrSI

	data["dataSize"] = len(dataArray)

	data["lastlng"] = lastlng
	data["lastlat"] = lastlat

	this.Data["json"] = data
	this.ServeJson()

}
