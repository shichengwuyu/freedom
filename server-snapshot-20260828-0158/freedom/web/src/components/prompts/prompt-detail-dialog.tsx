"use client";

import { Copy, ExternalLink, FolderPlus } from "lucide-react";
import { Button, Modal, Space, Tag } from "antd";

import { formatPromptDate, type Prompt } from "@/services/api/prompts";

export function PromptDetailDialog({ prompt, onClose, onCopy, onSaveAsset }: { prompt: Prompt | null; onClose: () => void; onCopy: (prompt: string) => void; onSaveAsset?: (prompt: Prompt) => void }) {
    const preview = prompt?.preview.replace(/!\[[^\]]*]\([^)]+\)/g, "").trim() || "";
    const isVideo = prompt?.category === "video" && prompt?.videoUrl;
    const hasExternalVideo = prompt?.category === "video" && prompt?.externalUrl && !prompt?.videoUrl;
    return (
        <>
            <Modal title={prompt?.title} open={Boolean(prompt)} onCancel={onClose} footer={null} width={860}>
                {prompt ? (
                    <>
                        <div className="grid gap-5 md:grid-cols-[300px_minmax(0,1fr)]">
                            <div className="space-y-3">
                                {isVideo ? (
                                    <video src={prompt.videoUrl} poster={prompt.coverUrl} className="aspect-[4/3] w-full rounded-lg object-cover" controls playsInline />
                                ) : (
                                    <a href={prompt.externalUrl || prompt.coverUrl} target="_blank" rel="noreferrer" className="block">
                                        <img src={prompt.coverUrl} alt={prompt.title} className="aspect-[4/3] w-full rounded-lg object-cover transition-opacity hover:opacity-80" />
                                    </a>
                                )}
                                {preview ? <pre className="max-h-60 overflow-auto whitespace-pre-wrap rounded-lg bg-stone-100 p-3 text-xs leading-5 text-stone-600 dark:bg-stone-900 dark:text-stone-300">{preview}</pre> : null}
                            </div>
                            <div className="min-w-0">
                                <div className="flex flex-wrap gap-1.5">
                                    {prompt.tags.map((tag) => (
                                        <Tag key={tag} className="m-0">
                                            {tag}
                                        </Tag>
                                    ))}
                                </div>
                                <p className="mt-4 whitespace-pre-wrap text-sm leading-7 text-stone-800 dark:text-stone-300">{prompt.prompt}</p>
                                <div className="mt-4 text-xs text-stone-500 dark:text-stone-400">
                                    创建：{formatPromptDate(prompt.createdAt)} · 更新：{formatPromptDate(prompt.updatedAt)}
                                </div>
                                <Space wrap className="mt-5">
                                    <Button type="primary" icon={<Copy className="size-4" />} onClick={() => onCopy(prompt.prompt)}>
                                        复制提示词
                                    </Button>
                                    {hasExternalVideo ? (
                                        <Button icon={<ExternalLink className="size-4" />} href={prompt.externalUrl} target="_blank" rel="noreferrer">
                                            在 YouMind 观看视频
                                        </Button>
                                    ) : null}
                                    {onSaveAsset ? (
                                        <Button icon={<FolderPlus className="size-4" />} onClick={() => onSaveAsset(prompt)}>
                                            加入我的素材
                                        </Button>
                                    ) : null}
                                </Space>
                            </div>
                        </div>
                    </>
                ) : null}
            </Modal>
        </>
    );
}
