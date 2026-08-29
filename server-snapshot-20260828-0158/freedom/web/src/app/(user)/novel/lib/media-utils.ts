import { downloadRemoteMedia, getMediaBlob } from "@/services/file-storage";

/** blob: URL → base64 data URL；非 blob: 原样返回 */
export async function blobUrlToDataUrl(blobUrl: string): Promise<string> {
    if (!blobUrl.startsWith("blob:")) return blobUrl;
    try {
        const response = await fetch(blobUrl);
        const blob = await response.blob();
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => resolve(reader.result as string);
            reader.onerror = () => reject(new Error("Failed to convert blob URL to data URL"));
            reader.readAsDataURL(blob);
        });
    } catch {
        return blobUrl; // 转换失败则返回原 URL
    }
}

/** 优先从本地 storageKey 读取视频 Blob，否则走远程代理下载 */
export async function getVideoBlob(videoUrl: string, storageKey?: string): Promise<Blob> {
    if (storageKey) {
        const local = await getMediaBlob(storageKey);
        if (local) return local;
    }
    return downloadRemoteMedia(videoUrl);
}

/** 从视频截取一帧作为图片（seekSeconds 可选：自定义截取位置秒数，默认按 type 选 0.1 / duration-0.1） */
export async function extractFrame(videoUrl: string, type: "first" | "last", storageKey?: string, seekSeconds?: number): Promise<string | null> {
    try {
        const blob = await getVideoBlob(videoUrl, storageKey);
        return await new Promise<string>((resolve, reject) => {
            const canvas = document.createElement("canvas");
            const video = document.createElement("video");
            const objUrl = URL.createObjectURL(blob);
            video.src = objUrl;
            video.crossOrigin = "anonymous";
            video.preload = "auto";
            video.onloadedmetadata = () => {
                const target = seekSeconds !== undefined
                    ? Math.max(0, Math.min(video.duration - 0.05, seekSeconds))
                    : type === "first" ? 0.1 : Math.max(0.1, video.duration - 0.1);
                video.currentTime = target;
            };
            video.onseeked = () => {
                canvas.width = video.videoWidth;
                canvas.height = video.videoHeight;
                canvas.getContext("2d")!.drawImage(video, 0, 0);
                const dataUrl = canvas.toDataURL("image/png");
                URL.revokeObjectURL(objUrl);
                resolve(dataUrl);
            };
            video.onerror = () => {
                URL.revokeObjectURL(objUrl);
                reject(new Error("视频帧提取失败"));
            };
        });
    } catch {
        return null;
    }
}
