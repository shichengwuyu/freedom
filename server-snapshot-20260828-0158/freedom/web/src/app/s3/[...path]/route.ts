import type { NextRequest } from "next/server";

// MinIO 对象存储读代理：把浏览器对 /s3/* 的请求转发到容器内 MinIO（http://minio:9000），
// 这样对象直链走网站 80 端口即可访问，无需对公网开放 MinIO 的 9000 端口。
export const runtime = "nodejs";
export const maxDuration = 300;

type RouteContext = {
    params: Promise<{ path: string[] }>;
};

function proxyHeaders(request: NextRequest) {
    const headers = new Headers(request.headers);
    headers.delete("host");
    headers.delete("content-length");
    headers.delete("connection");
    headers.delete("expect");
    return headers;
}

function responseHeaders(response: Response) {
    const headers = new Headers(response.headers);
    headers.delete("content-length");
    headers.delete("content-encoding");
    headers.delete("transfer-encoding");
    return headers;
}

async function proxy(request: NextRequest, context: RouteContext) {
    const { path } = await context.params;
    // 容器内直连 MinIO，端口无需对外暴露。可用 S3_INTERNAL_BASE_URL 覆盖。
    const baseUrl = process.env.S3_INTERNAL_BASE_URL || "http://minio:9000";
    const target = `${baseUrl.replace(/\/$/, "")}/${path.map(encodeURIComponent).join("/")}${request.nextUrl.search}`;
    const hasBody = request.method !== "GET" && request.method !== "HEAD";

    try {
        const response = await fetch(target, {
            method: request.method,
            headers: proxyHeaders(request),
            body: hasBody ? request.body : undefined,
            duplex: hasBody ? "half" : undefined,
            redirect: "manual",
        } as RequestInit & { duplex?: "half" });

        return new Response(response.body, {
            status: response.status,
            statusText: response.statusText,
            headers: responseHeaders(response),
        });
    } catch (error) {
        console.error("Failed to proxy s3", target, error);
        return Response.json({ code: 1, data: null, msg: "对象存储连接失败" }, { status: 502 });
    }
}

export const GET = proxy;
export const HEAD = proxy;
export const OPTIONS = proxy;
