package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

var adminModelHTTPClient = &http.Client{Timeout: 30 * time.Second}

func PublicSettings() (model.PublicSetting, error) {
	settings, err := repository.GetSettings()
	// normalizeSettings → normalizePublicSettingWithChannels 内部会通过 filterEnabledModels
	// 把私有渠道中所有启用模型与公开配置 availableModels 取并集后再过滤无效模型，
	// 因此无论 availableModels 是否为空，新增渠道的新模型都会自动对外可见。
	settings = normalizeSettings(settings)
	settings.Public.ModelChannel.Channels = publicChannelInfos(settings.Private.Channels)
	return settings.Public, err
}

func UserCanUseRemoteModelChannel(user model.AuthUser) bool {
	if user.Role == model.UserRoleAdmin {
		return true
	}
	settings, err := PublicSettings()
	return err == nil && settings.ModelChannel.AllowUserRemoteChannel != nil && *settings.ModelChannel.AllowUserRemoteChannel
}

func AdminSettings() (model.Settings, error) {
	settings, err := repository.GetSettings()
	return hidePrivateAPIKeys(normalizeSettings(settings)), err
}

func SaveSettings(settings model.Settings) (model.Settings, error) {
	saved, err := repository.GetSettings()
	if err != nil {
		return model.Settings{}, err
	}
	settings = normalizeSettings(settings)
	keepPrivateAPIKeys(&settings, normalizeSettings(saved))
	keepPrivateAuthSecrets(&settings, normalizeSettings(saved))
	keepPrivateStorageSecrets(&settings, normalizeSettings(saved))
	if err := validateEnabledStorageProviderTypes(settings.Private.Storage.Providers); err != nil {
		return model.Settings{}, err
	}
	result, err := repository.SaveSettings(settings, now())
	if err == nil {
		RefreshPromptSyncScheduler()
		RefreshStorageCapacityScheduler()
		RefreshAILogCleanupScheduler()
	}
	return hidePrivateAPIKeys(result), err
}

func AdminChannelModels(index *int, channel model.ModelChannel) ([]string, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return nil, err
	}
	return fetchAdminChannelModels(resolved)
}

func AdminTestChannelModel(index *int, channel model.ModelChannel, modelName string) (string, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return "", err
	}
	if isArkAgentPlanChannel(resolved) || isSeedanceModelName(modelName) {
		return testArkSeedanceChannelModel(resolved, modelName)
	}
	return testAdminChannelModel(resolved, modelName)
}

func normalizeSettings(settings model.Settings) model.Settings {
	settings.Private = normalizePrivateSetting(settings.Private)
	settings.Public = normalizePublicSettingWithChannels(settings.Public, settings.Private.Channels)
	return settings
}

func normalizePublicSetting(setting model.PublicSetting) model.PublicSetting {
	return normalizePublicSettingWithChannels(setting, nil)
}

func DefaultSystemPrompts() model.SystemPromptSetting {
	return model.SystemPromptSetting{
		Image:    "",
		Video:    "",
		Text:     "",
		Workflow: "",
		WorkflowAgent: `你是一个用于创建图片创作工作流的产品设计助理。请根据用户需求输出严格 JSON，不要输出 Markdown。
目标：把用户的自然语言需求整理为一个可复用的图片生成工作流。
要求：
1. 工作流必须面向同类型批量创作，变量字段要少而明确。
2. 变量名使用 snake_case，label 使用中文。
3. promptTemplate 必须使用 {{variable_name}} 引用变量。
4. 如果用户需要"多张、系列、组图、文章配图、海报组、写真组、方案集"，mode 使用 multi_image_series；否则使用 single_image。
5. config 只输出必要配置，apiMode 可为 responses 或 images。
6. variables 支持 text、textarea、number、select、boolean。
7. select 类型的 options 必须是字符串数组。
8. 多图工作流必须输出 seriesConfig，用于先生成多条图片提示词草稿。
9. 输出 JSON 结构：
{
  "name": "工作流名称",
  "category": "分类",
  "description": "一句话描述",
  "mode": "single_image",
  "variables": [
    {"key":"product_name","label":"产品名称","type":"text","required":true,"defaultValue":"","options":[]}
  ],
  "config": {
    "promptTemplate": "生成提示词模板",
    "systemPrompt": "系统提示词，可空",
    "model": "",
    "apiMode": "responses",
    "size": "auto",
    "quality": "auto",
    "count": "1",
    "outputFormat": "png",
    "timeout": 600
  },
  "seriesConfig": {
    "targetCount": "4",
    "promptInstruction": "多图拆分规则，可空",
    "reviewRequired": true,
    "concurrency": "3"
  },
  "warnings": []
}`,
		StoryboardScript: `你是一位专业的小说分镜改编导演。你的任务是把小说章节整合成一条完整的视频分镜描述词，供后续视频生成模型使用。

要求：
1. 总时长不超过 15 秒；用 0-Xs｜、Xs-Ys｜ 这样的时间段把镜头在同一条描述内部自然分层，每个时间段是一次运镜/一个机位，至少 2 个运镜段。
2. 开头单独两行标注「出场角色：…；场景：…」，随后用 { } 包裹按时间段展开的运镜描述。
3. 详细描述画面构图、人物位置关系、连贯的人物动作、台词、运镜、光影，按剧情推进自然衔接，承接上一章。
4. 台词零更改一字不动，说话/动作/表情前必须标注角色名；不要遗漏关键情节，也不要凭空添加剧本外的剧情。
5. 每个出场角色首次出现及其动作/台词前，必须用"@角色名"的形式标注，以便系统自动关联角色资产；场景不需要加@。
6. 角色外观、服装、场景描述必须严格参考上方【角色/场景/道具参考文档】中的描述，不可自行编造。
7. 只输出这一条完整的视频描述词本身，不要输出解释、总结或额外文字。

注意：这是"分镜剧本"，用于描述每一镜的画面内容，不是最终的视频生成提示词。`,
		StoryboardVideo: `我是一名即梦AI视频创作者，以上是我提供单镜头剧本内容和上一段视频描述词文档，和我提供的效果范文，和角色形象服装和场景参考文档，我需要你参考范文文档的效果，分析上一段的视频描述提示词把单镜头剧本内容以导演的思维延续上一段的视频提示词，把单镜头剧本内容转变为视频的描述词。角色形象服装和场景参考严格参考角色形象和场景文档中的描述，打斗战斗这些详细一些参考专门的效果参考
请你分析单镜头的剧本内容描述出，(出现的角色+场景)+｛xx秒多少次细节运镜提示词(运镜时间+画面构图+人物动作+人物台词+光影+整体运镜细节)+时间+固定模版(比例横屏16:9，总时长xx秒整，8K超高清，60帧高帧率，写实微电影质感，杜比视界HDR，广色域，光影层次丰富，无穿模、无畸变、无AI失真，口型与台词100%精准同步，情绪、微动作与台词语境完全匹配。视觉上追求电影级的东方美学光影。3D国漫GC风格，视频不出现字幕，视频不出现音乐BGM)｝详细效果参考效果范文文档
强调事项20个点：画面构图+人物动作
1.	画面构图包含：场景，人物站位，人物的姿势等等，场景描述的详细一点，比如场景在马路上，旁边有行人，有路灯，有车辆，人物姿势要描述详细一点，比如坐资，坐在沙发上，坐在椅子上，坐在驾驶位上等等根据文案描述详细一些
2.	每个分镜提示词内容中都要描述出角色的位置关系，不可省略，角色和场景简化一点，不超过100字
3.	人物动作：角色动作不能太单调，动作一定要连贯，要根据角色的状态变化
一些动作描述参考：转身离开，身体紧绷，轻轻摆手，手掌托住下巴，手指敲击桌面，眼睛眯起，脚步轻盈，稳步前行，快速奔跑，双手叉腰，翘着二郎腿，挠头，低头，摇头，仰头，皱眉，眨眼，握拳，拍手，转笔，弯腰，踮脚，踢腿，揉眼睛，摸下巴，双手抱胸，瘫坐，身体前倾，小碎步，脚步徘徊，叹气，撅嘴，摸肚子，摸耳朵，双手合十，单膝跪地，半蹲，从坐姿起身，靠着墙面坐下
注意：动作要连贯不能僵硬不动，比如：柳如烟缓慢走到沙发上坐下抬起腿翘着二郎腿一只手抬起拿起桌面上的水杯放在嘴边喝水眼神看向门口皱眉（动作一定要连贯）
3.运镜变化要顺畅不能太单调，每个分镜里面的运镜要详细对比上下分镜写出视频的丰富感，多搭配拍摄视角，视频景别，运镜变换，使得怎么视频精彩不枯燥
一些拍摄视角参考：远景，全景，中景，近景，特写，俯拍，平拍，仰拍，侧面拍摄，侧后拍摄，背面拍摄，正面拍摄，空景...等等，画面出现角色尽量减少远景，全景，中景描述
一些运镜参考：跟拍，平移，拉远，特写镜头，定格，推镜头，过肩镜头，反打镜头，空景，跳切镜头...等等
注意：运镜要符合逻辑，不能随便描述，表达出视频中的角色动作，情绪，氛围。
4.描述词避免出现色情和暴力的描述词
5.一定注意有台词不能省略，不能有一句台词有遗漏,也不可添加任何除了镜头内容外的台词文案，台词零更改： 所有台词将100%原文呈现，不增、不减、不改一字。
7.不出现任何以"我"或"主角"或"他"，"她"的描述词
8.谁做的动作，谁的表情，谁说的话前面角色名称不能省略，例如：柳清秋拿起手机，柳清秋表情冰冷，柳清秋说："等等我马上就到了"
9.注意角色位置描述不可以省
10.出现的角色和场景不能省略
11.镜头变化细节一些,运镜总时长在15秒以内，根据分镜内容的画面运镜，角色动作，角色说话台词等等调整好运镜的时长，特别是运镜中出现角色对话台词要根据台词的情绪和台词长度描述出符合的运镜时长。
12.根据单镜头内容以导演思维生成不可低于俩个运镜镜头
13.视频描述词节奏稍微快一点点，整个视频节奏不能太慢，动作要衔接流畅
14.描述词不能太简单，要特别细节一些
15. 注意台词的时长，台词每一个字给足0.4秒视频时长，例如台词10个字视频对应的运镜时长必须至少给足4秒时长
16.特别强调注意画面一定要连贯，不可出现卡顿，慢动作等等。动作连贯性： 每个角色的动作都经过精心设计，确保连贯、自然、不僵硬。
17.出现战斗的场景不可几秒结束，要多一些交战，给观众一些观感，爽感，比如:视角可用act视角镜头同步跟随出拳挥刀方向，打击瞬间自带顿针和镜头震动
18.强调只描述单镜头剧本内容，不可凭白无顾的出现情况剧本内容外的剧情，最后的提示词内容要和剧本内容一样，不能有其他剧情出现
19.再次说明只做上一段视频提示词的延续保证描述出来的提示词能和上一段的视频提示词连贯衔接，不可出现单镜头剧本内容外的剧情
20.视频提示词中出现的人物和场景要严格参考角色场景形象参考文档给的固定描述词`,
		StoryboardImage: `【角色三视图】纯白背景，无杂物。左侧为角色正面半身特写，右侧排列正面、左侧45度、侧面、背面四个全身视图，五视图身材高矮、五官脸型、肤色严格一致。聚焦角色本身：肌肤纹理细腻，五官精致对称，发丝/瞳孔/睫毛层次清晰；服装版型、缝线、配饰在每个视角保持统一；重心稳定、手指数正确、无畸变。3D古风角色立绘，国漫仙逆风格，高级CG建模，电影质感打光，伦勃朗光与柔光补光，明暗对比强烈，梦幻感与胶片质感融合，32K超清，大师级光影，杰出品质，商用/OC通用。
【场景四宫格】四宫格布局：{description}，同一地点四个固定视角——左上：正面远景（展示主入口/主立面），右上：左侧斜角（展示纵深），左下：右侧斜角（展示另一立面），右下：45度俯视（鸟瞰空间关系）。四个面板建筑结构、材质、植被、远景天际线保持一致；统一时段光线（日光/暮色/夜景任一）不混光；地面纹理、天空色调、季节氛围统一；空间纵深强，远中近景层次清晰。剔除人物与前景道具干扰，专心呈现环境氛围。3D古风场景概念，国漫仙逆风格，电影级场景氛围光，柔焦全焦远近清晰，32K超清，HDRI环境光，大师级纵深，杰出品质。
【道具标准图】纯白/纯灰渐变背景，无杂物、无强倒影、无地表。主物正面微3/4侧视角展示立体感，画面居中占比约60%。聚焦材质细节：金属高光、织物纹理、木纹年轮、玉石通透、玻璃折射、皮革毛孔清晰可辨；磨损、做旧、符文纹路、装饰刻线等工艺特征强化；色彩还原真实无夸张偏色。剔除人物与其他道具干扰，画面对称、留白适中。3D古风道具，国漫仙逆风格，产品级三维渲染，影棚三点布光，软阴影边缘，PBR材质精细呈现，32K超清，杰出品质。`,
	}
}

func normalizePublicSettingWithChannels(setting model.PublicSetting, channels []model.ModelChannel) model.PublicSetting {
	if setting.ModelChannel.AvailableModels == nil {
		setting.ModelChannel.AvailableModels = []string{}
	}
	if setting.ModelChannel.ModelCosts == nil {
		setting.ModelChannel.ModelCosts = []model.ModelCost{}
	}
	if setting.ModelChannel.Channels == nil {
		setting.ModelChannel.Channels = []model.PublicModelChannelInfo{}
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.Image) == "" {
		setting.ModelChannel.SystemPrompts.Image = firstNonEmpty(setting.ModelChannel.SystemPrompt, DefaultSystemPrompts().Image)
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.Video) == "" {
		setting.ModelChannel.SystemPrompts.Video = DefaultSystemPrompts().Video
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.Text) == "" {
		setting.ModelChannel.SystemPrompts.Text = firstNonEmpty(setting.ModelChannel.SystemPrompt, DefaultSystemPrompts().Text)
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.Workflow) == "" {
		setting.ModelChannel.SystemPrompts.Workflow = DefaultSystemPrompts().Workflow
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.WorkflowAgent) == "" {
		setting.ModelChannel.SystemPrompts.WorkflowAgent = DefaultSystemPrompts().WorkflowAgent
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.StoryboardScript) == "" {
		setting.ModelChannel.SystemPrompts.StoryboardScript = DefaultSystemPrompts().StoryboardScript
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.StoryboardVideo) == "" {
		setting.ModelChannel.SystemPrompts.StoryboardVideo = DefaultSystemPrompts().StoryboardVideo
	}
	if strings.TrimSpace(setting.ModelChannel.SystemPrompts.StoryboardImage) == "" {
		setting.ModelChannel.SystemPrompts.StoryboardImage = DefaultSystemPrompts().StoryboardImage
	}
	for i := range setting.ModelChannel.ModelCosts {
		setting.ModelChannel.ModelCosts[i].Model = strings.TrimSpace(setting.ModelChannel.ModelCosts[i].Model)
		setting.ModelChannel.ModelCosts[i].Label = strings.TrimSpace(setting.ModelChannel.ModelCosts[i].Label)
		if setting.ModelChannel.ModelCosts[i].CostCents < 0 {
			setting.ModelChannel.ModelCosts[i].CostCents = 0
		}
		if setting.ModelChannel.ModelCosts[i].CostCentsPerSecond < 0 {
			setting.ModelChannel.ModelCosts[i].CostCentsPerSecond = 0
		}
		if setting.ModelChannel.ModelCosts[i].Unit != model.ModelCostUnitPerSecond {
			setting.ModelChannel.ModelCosts[i].Unit = model.ModelCostUnitPerCall
		}
	}
	// 默认允许登录用户自定义模型渠道（可由管理员在后台关闭）
	// 默认开启普通用户使用云端渠道（只有这样才能统一扣 账户余额 金额）
	enabledRemote := true
	if setting.ModelChannel.AllowCustomChannel == nil {
		enabledCustom := true
		setting.ModelChannel.AllowCustomChannel = &enabledCustom
	}
	if setting.ModelChannel.AllowUserRemoteChannel == nil {
		setting.ModelChannel.AllowUserRemoteChannel = &enabledRemote
	}
	if setting.Auth.AllowRegister == nil {
		enabled := true
		setting.Auth.AllowRegister = &enabled
	}
	setting.ModelChannel.AvailableModels = filterEnabledModels(setting.ModelChannel.AvailableModels, enabledChannelModels(channels))
	if setting.SiteNotice.Contents == nil {
		setting.SiteNotice.Contents = []string{}
	}
	if strings.TrimSpace(setting.SiteNotice.Title) == "" {
		setting.SiteNotice.Title = "📢 网站公告"
	}
	setting.ContactSupport.WeChat = strings.TrimSpace(setting.ContactSupport.WeChat)
	setting.ContactSupport.QQ = strings.TrimSpace(setting.ContactSupport.QQ)
	setting.ContactSupport.WeChatQR = strings.TrimSpace(setting.ContactSupport.WeChatQR)
	setting.ContactSupport.QQGroup = strings.TrimSpace(setting.ContactSupport.QQGroup)
	setting.ContactSupport.QQGroupQR = strings.TrimSpace(setting.ContactSupport.QQGroupQR)
	setting.ContactSupport.Remark = strings.TrimSpace(setting.ContactSupport.Remark)
	return setting
}

func ModelCost(modelName string) (model.ModelCost, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelCost{}, err
	}
	modelName = strings.TrimSpace(modelName)
	for _, item := range normalizePublicSetting(settings.Public).ModelChannel.ModelCosts {
		if item.Model == modelName {
			return item, nil
		}
	}
	return model.ModelCost{Model: modelName, CostCents: 0, Unit: model.ModelCostUnitPerCall}, nil
}

func normalizePrivateSetting(setting model.PrivateSetting) model.PrivateSetting {
	if setting.Channels == nil {
		setting.Channels = []model.ModelChannel{}
	}
	setting.PromptSync = normalizePromptSyncSetting(setting.PromptSync)
	setting.AILog = normalizeAILogSetting(setting.AILog)
	setting.Storage = normalizePrivateStorageSetting(setting.Storage)
	for i := range setting.Channels {
		if setting.Channels[i].Protocol == "" {
			setting.Channels[i].Protocol = "openai"
		}
		if setting.Channels[i].ID == "" {
			setting.Channels[i].ID = stableModelChannelID(setting.Channels[i])
		}
		if setting.Channels[i].Models == nil {
			setting.Channels[i].Models = []string{}
		}
		if setting.Channels[i].Weight <= 0 {
			setting.Channels[i].Weight = 1
		}
		if setting.Channels[i].Timeout <= 0 {
			setting.Channels[i].Timeout = 600
		}
	}
	return setting
}

func hidePrivateAPIKeys(settings model.Settings) model.Settings {
	for i := range settings.Private.Channels {
		settings.Private.Channels[i].APIKey = ""
	}
	for i := range settings.Private.Storage.Providers {
		settings.Private.Storage.Providers[i].SecretAccessKey = ""
		settings.Private.Storage.Providers[i].Password = ""
	}
	settings.Private.Auth.LinuxDo.ClientSecret = ""
	return settings
}

func keepPrivateAPIKeys(settings *model.Settings, saved model.Settings) {
	for i := range settings.Private.Channels {
		if strings.TrimSpace(settings.Private.Channels[i].APIKey) != "" {
			continue
		}
		if channel, ok := findSavedChannel(settings.Private.Channels[i], saved.Private.Channels, i); ok {
			settings.Private.Channels[i].APIKey = channel.APIKey
		}
	}
}

func keepPrivateAuthSecrets(settings *model.Settings, saved model.Settings) {
	if strings.TrimSpace(settings.Private.Auth.LinuxDo.ClientSecret) == "" {
		settings.Private.Auth.LinuxDo.ClientSecret = saved.Private.Auth.LinuxDo.ClientSecret
	}
}

func findSavedChannel(channel model.ModelChannel, saved []model.ModelChannel, index int) (model.ModelChannel, bool) {
	for _, item := range saved {
		if item.Name == channel.Name && item.BaseURL == channel.BaseURL {
			return item, true
		}
	}
	if index >= 0 && index < len(saved) {
		return saved[index], true
	}
	return model.ModelChannel{}, false
}

func SelectModelChannel(modelName string) (model.ModelChannel, error) {
	return SelectModelChannelForModel(modelName, "")
}

func SelectModelChannelForModel(modelName string, channelID string) (model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelChannel{}, err
	}
	channels := modelChannelsForModel(normalizePrivateSetting(settings.Private).Channels, modelName)
	if len(channels) == 0 {
		return model.ModelChannel{}, errors.New("没有可用模型渠道")
	}
	if strings.TrimSpace(channelID) != "" {
		for _, channel := range channels {
			if channel.ID == channelID {
				return channel, nil
			}
		}
		return model.ModelChannel{}, errors.New("指定模型渠道不可用")
	}
	total := 0
	for _, channel := range channels {
		total += channel.Weight
	}
	hit := rand.Intn(total)
	for _, channel := range channels {
		hit -= channel.Weight
		if hit < 0 {
			return channel, nil
		}
	}
	return channels[0], nil
}

func HTTPClientForChannel(channel model.ModelChannel) *http.Client {
	timeout := channel.Timeout
	if timeout <= 0 {
		timeout = 600
	}
	return &http.Client{Timeout: time.Duration(timeout) * time.Second}
}

func BuildModelChannelURL(channel model.ModelChannel, path string) string {
	baseURL := normalizeModelChannelBaseURL(channel.BaseURL)
	lowerBaseURL := strings.ToLower(baseURL)
	if !strings.HasSuffix(lowerBaseURL, "/v1") && !strings.HasSuffix(lowerBaseURL, "/api/v3") && !strings.HasSuffix(lowerBaseURL, "/api/plan/v3") {
		baseURL += "/v1"
	}
	return baseURL + path
}

func normalizeModelChannelBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		path := strings.TrimRight(parsed.Path, "/")
		lowerPath := strings.ToLower(path)
		if index := strings.Index(lowerPath, "/api/plan/v3"); index >= 0 {
			end := index + len("/api/plan/v3")
			if len(lowerPath) == end || lowerPath[end] == '/' {
				parsed.Path = path[:end]
				parsed.RawPath = ""
				parsed.RawQuery = ""
				parsed.Fragment = ""
				return strings.TrimRight(parsed.String(), "/")
			}
		}
	}
	return baseURL
}

func isArkAgentPlanChannel(channel model.ModelChannel) bool {
	baseURL := strings.ToLower(normalizeModelChannelBaseURL(channel.BaseURL))
	return strings.HasSuffix(baseURL, "/api/plan/v3")
}

func isSeedanceModelName(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(modelName, "seedance") || strings.Contains(modelName, "doubao-seedance")
}

func enabledChannelModels(channels []model.ModelChannel) []string {
	models := []string{}
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		models = append(models, channel.Models...)
	}
	return uniqueModelNames(models)
}

func filterEnabledModels(models []string, options []string) []string {
	// 修复：把已有的 availableModels 与当前渠道启用模型取并集（自动追加新渠道的新模型），
	// 再过滤掉渠道中已不存在的模型（避免保留脏数据）。
	// 这样管理员新增渠道并保存后，新模型会自动合并进公开配置的 availableModels，
	// 无需再手动去公开配置里勾选一遍。
	allowed := map[string]bool{}
	for _, modelName := range options {
		allowed[modelName] = true
	}
	seen := map[string]bool{}
	result := []string{}
	// 先遍历并集：先保留原顺序的 models，再追加 options 中新增的模型
	for _, modelName := range uniqueModelNames(models) {
		if !allowed[modelName] || seen[modelName] {
			continue
		}
		seen[modelName] = true
		result = append(result, modelName)
	}
	for _, modelName := range uniqueModelNames(options) {
		if seen[modelName] {
			continue
		}
		seen[modelName] = true
		result = append(result, modelName)
	}
	return result
}

func uniqueModelNames(models []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range models {
		name := strings.TrimSpace(item)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

// isVideoModelName 判定一个模型名是否属于视频生成。
// 前端真相源是 web/src/lib/model-category.ts（VIDEO_KEYWORDS），
// 后端这里是镜像精简版（约 14 个关键词），保证 modelCosts / systemPrompts
// 等后端 routing 路径能看到一致的归类。任何 keyword 增删需同步 web/src/lib/model-category.ts。
func isVideoModelName(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	for _, kw := range []string{
		"video", "-video", "/video",
		"gemini-omni-flash", "gemini-omni-video",
		"seedance", "doubao-seedance",
		"kling", "hailuo", "sora", "veo",
		"wan/2-5", "wan/2-6", "wan/2-7",
		"wan2-5", "wan2-6", "wan2-7",
		"minimax", "happyhorse", "skyreels",
		"vidu", "pixverse", "runway",
	} {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

func isImageModelName(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(name, "seedream") || strings.Contains(name, "gpt-image") || strings.Contains(name, "image")
}

func isTextModelName(modelName string) bool {
	return !isImageModelName(modelName) && !isVideoModelName(modelName)
}

func normalizeModelChannel(channel model.ModelChannel) model.ModelChannel {
	if channel.Protocol == "" {
		channel.Protocol = "openai"
	}
	if channel.ID == "" {
		channel.ID = stableModelChannelID(channel)
	}
	if channel.Models == nil {
		channel.Models = []string{}
	}
	if channel.Weight <= 0 {
		channel.Weight = 1
	}
	if channel.Timeout <= 0 {
		channel.Timeout = 600
	}
	return channel
}

func resolveAdminChannel(index *int, channel model.ModelChannel) (model.ModelChannel, error) {
	resolved := normalizeModelChannel(channel)
	if strings.TrimSpace(resolved.APIKey) == "" {
		settings, err := repository.GetSettings()
		if err != nil {
			return model.ModelChannel{}, err
		}
		saved := normalizePrivateSetting(settings.Private).Channels
		if index != nil && *index >= 0 && *index < len(saved) {
			if resolved.APIKey == "" {
				resolved.APIKey = saved[*index].APIKey
			}
			if resolved.BaseURL == "" {
				resolved.BaseURL = saved[*index].BaseURL
			}
			if resolved.Name == "" {
				resolved.Name = saved[*index].Name
			}
		}
		if resolved.APIKey == "" {
			if savedChannel, ok := findSavedChannel(resolved, saved, -1); ok {
				resolved.APIKey = savedChannel.APIKey
			}
		}
	}
	if strings.TrimSpace(resolved.BaseURL) == "" {
		return model.ModelChannel{}, safeMessageError{message: "缺少接口地址"}
	}
	if strings.TrimSpace(resolved.APIKey) == "" {
		return model.ModelChannel{}, safeMessageError{message: "缺少 API Key"}
	}
	return resolved, nil
}

func fetchAdminChannelModels(channel model.ModelChannel) ([]string, error) {
	if IsMiMoChannel(channel) {
		result := MiMoModels()
		sort.Strings(result)
		return result, nil
	}
	if isKIEAdminChannel(channel) {
		result := kieMarketModels()
		sort.Strings(result)
		return result, nil
	}
	request, err := http.NewRequest(http.MethodGet, BuildModelChannelURL(channel, "/models"), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return nil, safeMessageError{message: "读取模型失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode >= http.StatusBadRequest {
		if response.StatusCode == http.StatusNotFound && isArkAgentPlanChannel(channel) {
			return nil, safeMessageError{message: "火山方舟 Agent Plan 未提供 OpenAI /models 模型列表接口，请手动填写模型名称，例如 doubao-seedance-2.0。"}
		}
		return nil, readAdminChannelError(body, response.StatusCode, "读取模型失败")
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &payload)
	result := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) != "" {
			result = append(result, item.ID)
		}
	}
	sort.Strings(result)
	return result, nil
}

func isKIEAdminChannel(channel model.ModelChannel) bool {
	protocol := strings.ToLower(strings.TrimSpace(channel.Protocol))
	baseURL := strings.ToLower(strings.TrimSpace(channel.BaseURL))
	return protocol == "kie" || strings.Contains(baseURL, "kie.ai")
}

func kieMarketModels() []string {
	return []string{
		"bytedance/seedream",
		"bytedance/seedream-v4-text-to-image",
		"bytedance/seedream-v4-edit",
		"seedream/4.5-text-to-image",
		"seedream/4.5-edit",
		"seedream/5-lite-text-to-image",
		"seedream/5-lite-image-to-image",
		"seedream/5-pro-text-to-image",
		"seedream/5-pro-image-to-image",
		"seedream/5-pro-layer-decomposition",
		"z-image",
		"nano-banana-2",
		"nano-banana-2-lite",
		"google/imagen4-fast",
		"google/imagen4-ultra",
		"google/imagen4",
		"google/nano-banana-edit",
		"google/nano-banana",
		"nano-banana-pro",
		"flux-2/pro-image-to-image",
		"flux-2/pro-text-to-image",
		"flux-2/flex-image-to-image",
		"flux-2/flex-text-to-image",
		"grok-imagine/text-to-image",
		"grok-imagine/image-to-image",
		"gpt-image/1.5-text-to-image",
		"gpt-image/1.5-image-to-image",
		"gpt-image-2-text-to-image",
		"gpt-image-2-image-to-image",
		"topaz/image-upscale",
		"recraft/remove-background",
		"recraft/crisp-upscale",
		"ideogram/character-edit",
		"ideogram/character-remix",
		"ideogram/character",
		"ideogram/v3-text-to-image",
		"ideogram/v3-edit",
		"ideogram/v3-remix",
		"qwen/text-to-image",
		"qwen/image-to-image",
		"qwen/image-edit",
		"qwen2/image-edit",
		"qwen2/text-to-image",
		"wan/2-7-image",
		"wan/2-7-image-pro",
		"grok-imagine/text-to-video",
		"grok-imagine/image-to-video",
		"grok-imagine/upscale",
		"grok-imagine/extend",
		"grok-imagine-video-1-5-preview",
		"minimax-h3/text-to-video",
		"minimax-h3/image-to-video",
		"minimax-h3/reference-to-video",
		"kling-2.6/text-to-video",
		"kling-2.6/image-to-video",
		"kling/v2-5-turbo-image-to-video-pro",
		"kling/v2-5-turbo-text-to-video-pro",
		"kling/ai-avatar-standard",
		"kling/ai-avatar-pro",
		"kling/v2-1-master-image-to-video",
		"kling/v2-1-master-text-to-video",
		"kling/v2-1-pro",
		"kling/v2-1-standard",
		"kling-2.6/motion-control",
		"kling-3.0/motion-control",
		"kling-3.0/video",
		"kling/v3-turbo-text-to-video",
		"kling/v3-turbo-image-to-video",
		"bytedance/seedance-2",
		"bytedance/seedance-2-fast",
		"bytedance/seedance-2-mini",
		"bytedance/seedance-1.5-pro",
		"bytedance/v1-pro-fast-image-to-video",
		"bytedance/v1-pro-image-to-video",
		"bytedance/v1-pro-text-to-video",
		"bytedance/v1-lite-image-to-video",
		"bytedance/v1-lite-text-to-video",
		"hailuo/2-3-image-to-video-pro",
		"hailuo/2-3-image-to-video-standard",
		"hailuo/02-text-to-video-pro",
		"hailuo/02-image-to-video-pro",
		"hailuo/02-text-to-video-standard",
		"hailuo/02-image-to-video-standard",
		"wan/2-2-a14b-image-to-video-turbo",
		"wan/2-2-a14b-speech-to-video-turbo",
		"wan/2-2-a14b-text-to-video-turbo",
		"wan/2-2-animate-move",
		"wan/2-2-animate-replace",
		"wan/2-6-image-to-video",
		"wan/2-6-text-to-video",
		"wan/2-6-video-to-video",
		"wan/2-6-flash-image-to-video",
		"wan/2-6-flash-video-to-video",
		"wan/2-5-image-to-video",
		"wan/2-5-text-to-video",
		"wan/2-7-text-to-video",
		"wan/2-7-image-to-video",
		"wan/2-7-videoedit",
		"wan/2-7-r2v",
		"topaz/video-upscale",
		"infinitalk/from-audio",
		"happyhorse/text-to-video",
		"happyhorse/image-to-video",
		"happyhorse/reference-to-video",
		"happyhorse/video-edit",
		"happyhorse-1-1/text-to-video",
		"happyhorse-1-1/image-to-video",
		"happyhorse-1-1/reference-to-video",
		"happyhorse-1-1/text-to-video",
		"happyhorse-1-1/image-to-video",
		"happyhorse-1-1/reference-to-video",
		"gemini-omni-video",
	}
}

func testAdminChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	if IsMiMoTTSModelName(modelName) {
		return testMiMoTTSChannelModel(channel, modelName)
	}
	body, _ := json.Marshal(map[string]any{
		"model": modelName,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "hi",
		}},
	})
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return "", safeMessageError{message: "测试失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode >= http.StatusBadRequest {
		return "", readAdminChannelError(responseBody, response.StatusCode, "测试失败")
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(responseBody, &payload)
	if len(payload.Choices) > 0 && strings.TrimSpace(payload.Choices[0].Message.Content) != "" {
		return payload.Choices[0].Message.Content, nil
	}
	return "ok", nil
}

func testMiMoTTSChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(modelName), "mimo-v2.5-tts-voiceclone") {
		return "MiMo VoiceClone 需要画布连接 MP3/WAV 参考音频，后台不发送克隆样本，因此未执行上游生成测试。", nil
	}
	messages := []map[string]string{{"role": "assistant", "content": "你好，这是语音模型测试。"}}
	audio := map[string]any{"format": "wav"}
	if strings.EqualFold(strings.TrimSpace(modelName), "mimo-v2.5-tts-voicedesign") {
		messages = append([]map[string]string{{"role": "user", "content": "自然清晰的年轻女声"}}, messages...)
	} else {
		audio["voice"] = "冰糖"
	}
	body, _ := json.Marshal(map[string]any{"model": modelName, "messages": messages, "audio": audio})
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := adminModelHTTPClient.Do(request)
	if err != nil {
		return "", safeMessageError{message: "测试失败：上游接口无响应或网络不可达"}
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode >= http.StatusBadRequest {
		return "", readAdminChannelError(responseBody, response.StatusCode, "测试失败")
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Audio *struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(responseBody, &payload) != nil || len(payload.Choices) == 0 || payload.Choices[0].Message.Audio == nil || strings.TrimSpace(payload.Choices[0].Message.Audio.Data) == "" {
		return "", safeMessageError{message: "测试失败：MiMo TTS 未返回音频数据"}
	}
	return "ok", nil
}

func testArkSeedanceChannelModel(channel model.ModelChannel, modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", errors.New("缺少模型名称")
	}
	if strings.TrimSpace(channel.BaseURL) == "" {
		return "", safeMessageError{message: "缺少接口地址"}
	}
	if strings.TrimSpace(channel.APIKey) == "" {
		return "", safeMessageError{message: "缺少 API Key"}
	}
	if !isArkAgentPlanChannel(channel) {
		return "Seedance 视频模型不会发送 /chat/completions 文本测试。已检查 Base URL、API Key 和模型名非空；未调用视频生成接口，因此未验证套餐额度或模型权限。", nil
	}
	return "Agent Plan / Seedance 视频模型配置格式已通过。后台测试不会调用视频生成接口，因此未验证 API Key、套餐额度或模型权限；请在画布中使用视频生成验证。", nil
}

func readAdminChannelError(body []byte, statusCode int, fallback string) error {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Msg string `json:"msg"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return safeMessageError{message: payload.Error.Message}
		}
		if strings.TrimSpace(payload.Msg) != "" {
			return safeMessageError{message: payload.Msg}
		}
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return safeMessageError{message: fmt.Sprintf("上游接口鉴权失败（%d），请检查 API Key、套餐权限或模型权限", statusCode)}
	}
	if statusCode == http.StatusTooManyRequests {
		return safeMessageError{message: "上游接口限流或额度不足（429），请稍后重试或检查额度"}
	}
	if statusCode > 0 {
		return safeMessageError{message: fmt.Sprintf("%s：%d", fallback, statusCode)}
	}
	return safeMessageError{message: fallback}
}

type safeMessageError struct {
	message string
}

func (err safeMessageError) Error() string {
	return err.message
}

func (err safeMessageError) SafeMessage() string {
	return err.message
}

func keepPrivateStorageSecrets(settings *model.Settings, saved model.Settings) {
	for i := range settings.Private.Storage.Providers {
		current := &settings.Private.Storage.Providers[i]
		if strings.TrimSpace(current.SecretAccessKey) != "" && strings.TrimSpace(current.Password) != "" {
			continue
		}
		if provider, ok := findSavedStorageProvider(*current, saved.Private.Storage.Providers, i); ok {
			if strings.TrimSpace(current.SecretAccessKey) == "" {
				current.SecretAccessKey = provider.SecretAccessKey
			}
			if strings.TrimSpace(current.Password) == "" {
				current.Password = provider.Password
			}
		}
	}
}

func findSavedStorageProvider(provider model.StorageProvider, saved []model.StorageProvider, index int) (model.StorageProvider, bool) {
	for _, item := range saved {
		if provider.ID != "" && item.ID == provider.ID {
			return item, true
		}
		if item.Type == provider.Type && item.Name == provider.Name && item.Endpoint == provider.Endpoint && item.Bucket == provider.Bucket && (provider.Type != model.StorageProviderTypeWebDAV || item.PathPrefix == provider.PathPrefix) {
			return item, true
		}
	}
	if index >= 0 && index < len(saved) && saved[index].Type == provider.Type {
		return saved[index], true
	}
	return model.StorageProvider{}, false
}

func validateEnabledStorageProviderTypes(providers []model.StorageProvider) error {
	enabledType := ""
	for _, provider := range providers {
		if provider.Type != model.StorageProviderTypeS3 && provider.Type != model.StorageProviderTypeWebDAV && provider.Type != model.StorageProviderTypeLocal {
			return safeMessageError{message: "存储类型不支持"}
		}
		if !provider.Enabled {
			continue
		}
		if enabledType == "" {
			enabledType = provider.Type
			continue
		}
		if enabledType != provider.Type {
			return safeMessageError{message: "只能启用一种存储类型"}
		}
	}
	return nil
}

func normalizePrivateStorageSetting(setting model.PrivateStorageSetting) model.PrivateStorageSetting {
	if setting.Mode == "" {
		setting.Mode = "local_indexeddb"
		setting.AllowUserGlobalProvider = true
	}
	if setting.CapacityLimitBytes <= 0 {
		setting.CapacityLimitBytes = 9 * 1024 * 1024 * 1024
	}
	setting.CapacityCheck = normalizeStorageCapacityCheckSetting(setting.CapacityCheck)
	if setting.Providers == nil {
		setting.Providers = []model.StorageProvider{}
	}
	for i := range setting.Providers {
		setting.Providers[i] = normalizeStorageProvider(setting.Providers[i])
	}
	// 环境变量注入的默认 S3 存储：部署即生效，去重后追加（不覆盖后台手动配置）。
	if envProvider := envStorageProvider(); envProvider != nil {
		exists := false
		for _, p := range setting.Providers {
			if p.Endpoint == envProvider.Endpoint && p.Bucket == envProvider.Bucket {
				exists = true
				break
			}
		}
		if !exists {
			setting.Providers = append(setting.Providers, *envProvider)
		}
	}
	return setting
}

// envStorageProvider 从环境变量构造默认的 S3 兼容对象存储 provider（如 MinIO）。
// 部署时在 .env 配一次 STORAGE_S3_* 即可自动生效，无需每次手动进后台配置。
func envStorageProvider() *model.StorageProvider {
	cfg := config.Cfg
	if strings.TrimSpace(cfg.StorageS3Endpoint) == "" || strings.TrimSpace(cfg.StorageS3Bucket) == "" {
		return nil
	}
	provider := normalizeStorageProvider(model.StorageProvider{
		Name:            "环境变量存储 (STORAGE_S3_*)",
		Type:            model.StorageProviderTypeS3,
		Endpoint:        strings.TrimRight(strings.TrimSpace(cfg.StorageS3Endpoint), "/"),
		Region:          strings.TrimSpace(cfg.StorageS3Region),
		Bucket:          strings.TrimSpace(cfg.StorageS3Bucket),
		AccessKeyID:     strings.TrimSpace(cfg.StorageS3AccessKey),
		SecretAccessKey: cfg.StorageS3SecretKey,
		PublicBaseURL:   strings.TrimSpace(cfg.StorageS3PublicURL),
		Weight:          1,
		Enabled:         true,
	})
	return &provider
}

func normalizeStorageCapacityCheckSetting(setting model.StorageCapacityCheckSetting) model.StorageCapacityCheckSetting {
	if setting.Cron == "" {
		setting.Cron = "0 */6 * * *"
	}
	if setting.Enabled == nil {
		enabled := false
		setting.Enabled = &enabled
	}
	return setting
}

func normalizeStorageProvider(provider model.StorageProvider) model.StorageProvider {
	provider.Name = strings.TrimSpace(provider.Name)
	provider.Type = strings.ToLower(strings.TrimSpace(provider.Type))
	if provider.Type == "" {
		provider.Type = model.StorageProviderTypeS3
	}
	// local 类型不需要 Endpoint 校验
	if provider.Type != model.StorageProviderTypeLocal {
		provider.Endpoint = strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/")
	} else {
		provider.Endpoint = strings.TrimSpace(provider.Endpoint)
	}
	provider.Bucket = strings.TrimSpace(provider.Bucket)
	provider.AccessKeyID = strings.TrimSpace(provider.AccessKeyID)
	if provider.Type == model.StorageProviderTypeWebDAV {
		provider.PathPrefix = strings.Trim(strings.TrimSpace(provider.PathPrefix), "/")
		if provider.PathPrefix == "" {
			provider.PathPrefix = "canvas"
		}
	}
	if provider.Type == model.StorageProviderTypeS3 {
		provider.Username = strings.TrimSpace(provider.Username)
	}
	if provider.Type == model.StorageProviderTypeS3 && provider.Region == "" {
		provider.Region = "auto"
	}
	if provider.ID == "" {
		provider.ID = stableStorageProviderID(provider)
	}
	if provider.Weight <= 0 {
		provider.Weight = 1
	}
	return provider
}

func stableStorageProviderID(provider model.StorageProvider) string {
	webDAVPath := ""
	if provider.Type == model.StorageProviderTypeWebDAV {
		webDAVPath = provider.PathPrefix
	}
	return "storage-" + providerSecureHash([]string{provider.OwnerUserID, provider.Type, provider.Name, provider.Endpoint, provider.Bucket, webDAVPath})
}

func stableModelChannelID(channel model.ModelChannel) string {
	return "channel-" + providerSecureHash([]string{channel.Name, channel.BaseURL})
}

func providerSecureHash(parts []string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])[:16]
}

func modelChannelsForModel(channels []model.ModelChannel, modelName string) []model.ModelChannel {
	result := []model.ModelChannel{}
	for _, channel := range channels {
		if !channel.Enabled || channel.BaseURL == "" || channel.APIKey == "" {
			continue
		}
		for _, item := range channel.Models {
			if strings.TrimSpace(item) == modelName {
				result = append(result, channel)
				break
			}
		}
	}
	return result
}

func publicChannelInfos(channels []model.ModelChannel) []model.PublicModelChannelInfo {
	result := []model.PublicModelChannelInfo{}
	for _, channel := range channels {
		if !channel.Enabled || channel.BaseURL == "" || len(channel.Models) == 0 {
			continue
		}
		result = append(result, model.PublicModelChannelInfo{
			ID:          channel.ID,
			Name:        channel.Name,
			BaseURL:     channel.BaseURL,
			Models:      append([]string{}, channel.Models...),
			ModelLabels: copyModelLabels(channel.ModelLabels),
			Weight:      channel.Weight,
			Timeout:     channel.Timeout,
			Enabled:     channel.Enabled,
			Remark:      channel.Remark,
		})
	}
	return result
}

// copyModelLabels 复制模型别名映射，避免共享底层 map。
func copyModelLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	return result
}

func collectChannelModels(channels []model.ModelChannel) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, channel := range channels {
		if !channel.Enabled || channel.BaseURL == "" {
			continue
		}
		for _, item := range channel.Models {
			modelName := strings.TrimSpace(item)
			if modelName == "" || seen[modelName] {
				continue
			}
			seen[modelName] = true
			result = append(result, modelName)
		}
	}
	sort.Strings(result)
	return result
}
