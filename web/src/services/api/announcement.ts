import { apiGet, apiPost, compactApiParams } from "@/services/api/request";

export type AnnouncementItem = {
    id: string;
    content: string;
    createdAt: string;
    updatedAt: string;
};

export type AnnouncementListResponse = {
    items: AnnouncementItem[];
    total: number;
};

export type LatestAnnouncementsResponse = {
    items: AnnouncementItem[];
};

export type AdminAnnouncementQuery = {
    page?: number;
    pageSize?: number;
    keyword?: string;
};

// 公共接口：获取最新公告（最多10条）
export async function getLatestAnnouncements() {
    return apiGet<LatestAnnouncementsResponse>("/api/announcements/latest");
}

// 管理接口：公告列表
export async function adminListAnnouncements(token: string, query: AdminAnnouncementQuery = {}) {
    return apiGet<AnnouncementListResponse>("/api/admin/announcements", compactApiParams(query), token);
}

// 管理接口：新增/保存公告
export async function adminSaveAnnouncement(
    token: string,
    body: { id?: string; content: string },
) {
    return apiPost<null>("/api/admin/announcements", body, token);
}

// 管理接口：删除公告
export async function adminDeleteAnnouncement(token: string, id: string) {
    return apiPost<null>("/api/admin/announcements/delete", { id }, token);
}
