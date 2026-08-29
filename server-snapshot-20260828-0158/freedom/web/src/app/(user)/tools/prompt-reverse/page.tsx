"use client";

import { ArrowLeft, Upload, Settings2, RotateCcw } from "lucide-react";
import { useState, useEffect } from "react";
import Link from "next/link";
import { Button, Card, Upload as AntUpload, message, InputNumber, Collapse } from "antd";
import type { UploadFile, RcFile } from "antd/es/upload/interface";
import localforage from "localforage";
import { aiApiUrl, aiHeaders } from "@/services/api/image";
import { useEffectiveConfig, useConfigStore } from "@/stores/use-config-store";
import { ModelPicker } from "@/components/model-picker";

// 视频反推系统提示词（拉片分析）
const VIDEO_SYSTEM_PROMPT = `# 角色设定

你现在是一位资深的影视视频分析专家和顶级 AI 视频生成模型（如 Veo、Sora 等）的提示词工程师。

你拥有敏锐的视觉和听觉洞察力，擅长将复杂的视频画面解构为专业的影视语言，并能将其转化为 AI 能够理解的高质量视频生成提示词（Prompt）。

# 任务目标

当我上传一段视频后，你需要仔细观察视频的画面、运镜、角色、光影、色彩和音频，对视频进行专业的"拉片"分析，并反推出可用于 AI 视频生成的详细结构化笔记。

重点要求：

由于 AI 视频生成通常以 15 秒作为一个生成周期，请严格按照【每 {interval} 秒一个大节点】进行拆分分析。

每个节点需要独立分析：
- 镜头变化
- 人物动作发展
- 画面细节
- 运镜方式
- 光影变化
- 声音设计

确保分析结果能够直接用于 Veo、Sora 等 AI 视频模型生成对应的视频片段。

# 输出格式要求

请严格按照以下结构输出分析结果，保持排版清晰整洁。

# 1. 专家简评

以视频分析专家的口吻，用一到两句话简述：
- 视频核心特点
- 故事基调
- 整体视觉风格
- AI 视频生成难点

# 2. 视频提示词（拉片笔记）

使用表格形式，按照时间节点详细记录。

表格必须包含以下列：
| 镜号 | 时间轴 | 景别/角度 | 运动 | 画面内容 | 音频 |

# 3. AI 视频生成 Prompt 总结

根据以上拉片分析，最后输出一版可以直接用于 AI 视频生成模型的完整 Prompt。

Prompt 必须包含：
- 场景
- 人物
- 动作
- 镜头语言
- 运镜方式
- 光影
- 色彩
- 特效
- 音频氛围

# 4. Negative Prompt（负面提示词）

最后输出适用于 AI 视频生成的负面提示词。`;

// 图片反推系统提示词
const IMAGE_SYSTEM_PROMPT = `# 角色设定

你现在是一位顶级的 AI 图像生成提示词工程师，擅长分析图片的视觉元素并转化为高质量的 AI 绘图提示词。

# 任务目标

当我上传一张图片后，你需要仔细观察图片的构图、色彩、光影、风格、主体、细节和氛围，反推出可用于 AI 图像生成模型（如 Midjourney、DALL-E、Stable Diffusion 等）的详细提示词。

# 输出格式要求

请严格按照以下结构输出分析结果：

# 1. 画面简评

用一到两句话简述：
- 图片核心特点
- 整体视觉风格
- 色彩与光影基调

# 2. 详细描述

从以下维度逐项分析：
- 主体描述（人物/物体/场景）
- 构图与视角
- 光影与色彩
- 艺术风格
- 细节特征
- 氛围与情绪

# 3. AI 图像生成 Prompt

根据以上分析，输出一版可以直接用于 AI 图像生成模型的完整 Prompt。

Prompt 必须包含：
- 主体描述
- 场景环境
- 构图视角
- 光影效果
- 色彩方案
- 艺术风格
- 画质关键词
- 细节补充

# 4. Negative Prompt（负面提示词）

最后输出适用于 AI 图像生成的负面提示词。`;

export default function PromptReversePage() {
    const config = useEffectiveConfig();
    const updateConfig = useConfigStore((state) => state.updateConfig);
    const openConfigDialog = useConfigStore((state) => state.openConfigDialog);
    const [fileList, setFileList] = useState<UploadFile[]>([]);
    const [loading, setLoading] = useState(false);
    const [result, setResult] = useState<string>("");
    const [interval, setInterval] = useState<number>(15);
    const [customSystemPrompt, setCustomSystemPrompt] = useState<string>("");
    const [promptEdited, setPromptEdited] = useState(false);
    const [fileType, setFileType] = useState<"image" | "video">("image");

    // 选中的文本模型，默认用 config.textModel
    const [selectedModel, setSelectedModel] = useState<string>(config.textModel || "");
    const [selectedChannelId, setSelectedChannelId] = useState<string>(config.textChannelId || "");

    // 当 config.textModel 变化时同步默认选中
    useEffect(() => {
        if (config.textModel && !selectedModel) {
            setSelectedModel(config.textModel);
            setSelectedChannelId(config.textChannelId || "");
        }
    }, [config.textModel, config.textChannelId, selectedModel]);

    // 持久化选中的文本模型到全局配置
    const handleModelChange = (model: string, channelId?: string) => {
        setSelectedModel(model);
        setSelectedChannelId(channelId || "");
        updateConfig("textModel", model);
        if (channelId) updateConfig("textChannelId", channelId);
    };

    // 从 localforage 加载用户自定义提示词
    useEffect(() => {
        localforage.getItem<string>("tools:prompt-reverse-system-prompt").then((saved) => {
            if (saved) {
                setCustomSystemPrompt(saved);
                setPromptEdited(true);
            }
        });
    }, []);

    // 保存用户自定义提示词到 localforage
    const handlePromptChange = (value: string) => {
        setCustomSystemPrompt(value);
        setPromptEdited(value !== (fileType === "video" ? VIDEO_SYSTEM_PROMPT : IMAGE_SYSTEM_PROMPT));
        if (value.trim()) {
            localforage.setItem("tools:prompt-reverse-system-prompt", value);
        } else {
            localforage.removeItem("tools:prompt-reverse-system-prompt");
        }
    };

    // 恢复默认提示词
    const handleResetPrompt = () => {
        setCustomSystemPrompt("");
        setPromptEdited(false);
        localforage.removeItem("tools:prompt-reverse-system-prompt");
        message.success("已恢复默认提示词");
    };

    // 当前生效的系统提示词（用户自定义 > 默认，根据文件类型选择）
    const activeDefaultPrompt = fileType === "video" ? VIDEO_SYSTEM_PROMPT : IMAGE_SYSTEM_PROMPT;

    const handleUpload = async (file: RcFile) => {
        // 根据文件类型设置 fileType
        const isVideo = file.type.startsWith("video/");
        setFileType(isVideo ? "video" : "image");

        setLoading(true);
        setResult("");

        try {
            // 读取文件为 base64
            const reader = new FileReader();
            reader.onload = async () => {
                const base64 = reader.result as string;

                // 使用用户自定义提示词或默认提示词（视频模板需替换 {interval}）
                const basePrompt = customSystemPrompt || (isVideo ? VIDEO_SYSTEM_PROMPT : IMAGE_SYSTEM_PROMPT);
                const systemPrompt = isVideo ? basePrompt.replace("{interval}", String(interval)) : basePrompt;

                // 构建消息内容：图片用 image_url，视频用 video_url
                const contentType = isVideo ? "video_url" : "image_url";
                const contentKey = isVideo ? "video_url" : "image_url";
                const messages = [
                    { role: "system", content: systemPrompt },
                    {
                        role: "user",
                        content: [
                            { type: contentType, [contentKey]: { url: base64 } } as Record<string, unknown>,
                            {
                                type: "text",
                                text: isVideo
                                    ? `请分析这段视频，按照每 ${interval} 秒一个节点进行拆分分析，输出完整的拉片笔记和 AI 视频生成提示词。`
                                    : `请分析这张图片，输出详细的描述和 AI 图像生成提示词。`,
                            },
                        ],
                    },
                ];

                // 构建请求配置：使用文本模型和文本渠道
                const requestConfig = {
                    ...config,
                    model: selectedModel || config.textModel,
                    activeChannelId: selectedChannelId || config.textChannelId,
                };

                // 调用 AI API
                const response = await fetch(aiApiUrl(requestConfig, "/chat/completions"), {
                    method: "POST",
                    headers: aiHeaders(requestConfig, "application/json"),
                    body: JSON.stringify({
                        model: selectedModel || config.textModel,
                        messages,
                        stream: false,
                    }),
                });

                if (!response.ok) {
                    const errorText = await response.text().catch(() => "");
                    throw new Error(`API 请求失败: ${response.status}${errorText ? ` - ${errorText}` : ""}`);
                }

                const data = await response.json();
                // 兼容多种响应格式
                const content =
                    data.choices?.[0]?.message?.content ||
                    data.data?.choices?.[0]?.message?.content ||
                    "";

                if (!content) {
                    throw new Error("AI 没有返回有效内容，请检查模型是否支持图片/视频输入");
                }

                setResult(content);
                setLoading(false);
                message.success("分析完成");
            };

            reader.onerror = () => {
                setLoading(false);
                message.error("文件读取失败");
            };

            reader.readAsDataURL(file);
        } catch (error) {
            setLoading(false);
            message.error(error instanceof Error ? error.message : "分析失败");
        }

        return false; // 阻止自动上传
    };

    return (
        <div className="mx-auto max-w-4xl px-6 py-8">
            {/* 顶部导航：返回 + 标题 */}
            <div className="mb-6 flex items-center gap-3">
                <Link href="/tools">
                    <Button type="text" icon={<ArrowLeft className="size-4" />} className="text-stone-600 dark:text-stone-300">
                        返回
                    </Button>
                </Link>
                <h1 className="text-2xl font-semibold text-stone-950 dark:text-stone-100">提示词反推</h1>
            </div>

            <Card>
                <p className="mb-4 text-sm text-stone-500 dark:text-stone-400">
                    上传图片或视频，AI 自动分析并生成描述提示词
                </p>

                {/* 模型选择器 */}
                <div className="mb-4 flex items-center gap-3">
                    <span className="text-sm text-stone-700 dark:text-stone-300">分析模型：</span>
                    <ModelPicker
                        config={config}
                        value={selectedModel || config.textModel}
                        channelId={selectedChannelId || config.textChannelId}
                        capability="text"
                        onChange={handleModelChange}
                        onMissingConfig={() => openConfigDialog(true)}
                        placeholder="选择文本模型"
                    />
                </div>

                {/* 视频分析间隔设置（仅视频时显示） */}
                {fileType === "video" && (
                    <div className="mb-4 flex items-center gap-3">
                        <span className="text-sm text-stone-700 dark:text-stone-300">分析间隔：</span>
                        <InputNumber
                            min={5}
                            max={60}
                            value={interval}
                            onChange={(value) => setInterval(value || 15)}
                            addonAfter="秒"
                            className="w-32"
                        />
                        <span className="text-xs text-stone-500">默认 15 秒一个分析节点</span>
                    </div>
                )}

                {/* 系统提示词配置 */}
                <Collapse
                    className="mb-4"
                    items={[
                        {
                            key: "1",
                            label: (
                                <div className="flex items-center gap-2">
                                    <Settings2 className="size-4" />
                                    <span>系统提示词配置</span>
                                    {promptEdited && (
                                        <span className="rounded bg-blue-100 px-1.5 py-0.5 text-xs text-blue-700 dark:bg-blue-900 dark:text-blue-300">
                                            已自定义
                                        </span>
                                    )}
                                </div>
                            ),
                            children: (
                                <div className="space-y-3">
                                    <div className="flex items-center justify-between">
                                        <span className="text-xs text-stone-500">
                                            {fileType === "video"
                                                ? "视频提示词中的 {interval} 会自动替换为上方设置的分析间隔秒数"
                                                : "图片提示词无需替换变量"}
                                        </span>
                                        {promptEdited && (
                                            <Button
                                                size="small"
                                                icon={<RotateCcw className="size-3" />}
                                                onClick={handleResetPrompt}
                                            >
                                                恢复默认
                                            </Button>
                                        )}
                                    </div>
                                    <textarea
                                        value={customSystemPrompt || activeDefaultPrompt}
                                        onChange={(e) => handlePromptChange(e.target.value)}
                                        className="w-full rounded-lg border border-stone-300 bg-white p-3 font-mono text-xs text-stone-700 focus:border-blue-500 focus:outline-none dark:border-stone-600 dark:bg-stone-800 dark:text-stone-300"
                                        rows={12}
                                        placeholder="输入自定义系统提示词..."
                                    />
                                </div>
                            ),
                        },
                    ]}
                />

                {/* 上传区域 */}
                <AntUpload.Dragger
                    accept="image/*,video/*"
                    fileList={fileList}
                    beforeUpload={handleUpload}
                    onChange={({ fileList: newFileList }) => setFileList(newFileList)}
                    maxCount={1}
                    showUploadList={{ showPreviewIcon: true, showRemoveIcon: true }}
                >
                    <p className="ant-upload-drag-icon">
                        <Upload className="mx-auto size-12 text-stone-400" />
                    </p>
                    <p className="ant-upload-text">点击或拖拽文件到此区域</p>
                    <p className="ant-upload-hint">
                        支持图片（JPG、PNG、WebP）和视频（MP4、MOV）格式
                    </p>
                </AntUpload.Dragger>

                {/* 加载提示 */}
                {loading && (
                    <div className="mt-6 flex items-center justify-center py-8">
                        <div className="text-center">
                            <div className="mb-2 text-sm text-stone-500">AI 正在分析中...</div>
                            <div className="text-xs text-stone-400">分析可能需要几分钟，请耐心等待</div>
                        </div>
                    </div>
                )}

                {/* 结果展示 */}
                {result && (
                    <div className="mt-6">
                        <div className="mb-2 flex items-center justify-between">
                            <span className="text-sm font-medium text-stone-700 dark:text-stone-300">
                                生成的提示词
                            </span>
                            <Button
                                size="small"
                                onClick={() => {
                                    navigator.clipboard.writeText(result);
                                    message.success("已复制到剪贴板");
                                }}
                            >
                                复制
                            </Button>
                        </div>
                        <div className="rounded-lg border border-stone-200 bg-stone-50 p-4 dark:border-stone-700 dark:bg-stone-800">
                            <p className="whitespace-pre-wrap text-sm text-stone-700 dark:text-stone-300">
                                {result}
                            </p>
                        </div>
                    </div>
                )}
            </Card>
        </div>
    );
}
