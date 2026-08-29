"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { App } from "antd";

import { approveAdminPrompt, deleteAdminPrompt, deleteAdminPrompts, fetchAdminPendingPrompts, fetchAdminPrompts, fetchAdminPromptCategories, fetchAdminRejectedPrompts, rejectAdminPrompt, saveAdminPrompt, syncAdminPromptCategoriesAll, syncAdminPromptCategory, type AdminPromptCategory } from "@/services/api/admin";
import type { Prompt } from "@/services/api/prompts";
import { useUserStore } from "@/stores/use-user-store";

const defaultPageSize = 10;

export type AdminPromptTab = "all" | "pending" | "rejected";

export function useAdminPrompts() {
    const { message } = App.useApp();
    const queryClient = useQueryClient();
    const token = useUserStore((state) => state.token);
    const clearSession = useUserStore((state) => state.clearSession);
    const [keyword, setKeyword] = useState("");
    const [category, setCategory] = useState("");
    const [tag, setTag] = useState<string[]>([]);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(defaultPageSize);
    const [activeTab, setActiveTab] = useState<AdminPromptTab>("all");

    const categoriesQuery = useQuery({
        queryKey: ["admin", "prompt-categories", token],
        queryFn: () => fetchAdminPromptCategories(token),
        enabled: Boolean(token),
        retry: false,
    });

    const promptsQuery = useQuery({
        queryKey: ["admin", "prompts", token, keyword, category, tag, page, pageSize],
        queryFn: () => fetchAdminPrompts(token, { keyword, category, tag, page, pageSize }),
        enabled: Boolean(token) && activeTab === "all",
        retry: false,
    });

    const pendingQuery = useQuery({
        queryKey: ["admin", "prompt-pending", token, page, pageSize],
        queryFn: () => fetchAdminPendingPrompts(token, page, pageSize),
        enabled: Boolean(token) && activeTab === "pending",
        retry: false,
    });

    const rejectedQuery = useQuery({
        queryKey: ["admin", "prompt-rejected", token, page, pageSize],
        queryFn: () => fetchAdminRejectedPrompts(token, page, pageSize),
        enabled: Boolean(token) && activeTab === "rejected",
        retry: false,
    });

    const syncMutation = useMutation({
        mutationFn: (category: string) => syncAdminPromptCategory(token, category),
        onSuccess: async (categories) => {
            queryClient.setQueryData<AdminPromptCategory[]>(["admin", "prompt-categories", token], categories);
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompts"] });
            message.success("远程提示词源已同步");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "同步失败");
        },
    });

    const syncAllMutation = useMutation({
        mutationFn: () => syncAdminPromptCategoriesAll(token),
        onSuccess: async (categories) => {
            queryClient.setQueryData<AdminPromptCategory[]>(["admin", "prompt-categories", token], categories);
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompts"] });
            message.success("全部远程提示词源已同步");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "同步失败");
        },
    });

    const approveMutation = useMutation({
        mutationFn: (id: string) => approveAdminPrompt(token, id),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompt-pending"] });
            message.success("已通过审核");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "操作失败");
        },
    });

    const rejectMutation = useMutation({
        mutationFn: (id: string) => rejectAdminPrompt(token, id),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompt-rejected"] });
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompt-pending"] });
            message.success("已拒绝");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "操作失败");
        },
    });

    const saveMutation = useMutation({
        mutationFn: (prompt: Partial<Prompt>) => saveAdminPrompt(token, prompt),
        onSuccess: async (_, prompt) => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompt-categories"] });
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompts"] });
            message.success(prompt.id ? "提示词已保存" : "提示词已新增");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "保存失败");
        },
    });

    const deleteMutation = useMutation({
        mutationFn: (id: string) => deleteAdminPrompt(token, id),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompt-categories"] });
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompts"] });
            message.success("提示词已删除");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "删除失败");
        },
    });

    const batchDeleteMutation = useMutation({
        mutationFn: (ids: string[]) => deleteAdminPrompts(token, ids),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompt-categories"] });
            await queryClient.invalidateQueries({ queryKey: ["admin", "prompts"] });
            message.success("提示词已批量删除");
        },
        onError: (error) => {
            message.error(error instanceof Error ? error.message : "批量删除失败");
        },
    });

    useEffect(() => {
        const error = categoriesQuery.error || promptsQuery.error || pendingQuery.error || rejectedQuery.error;
        if (!error) return;
        const errorMessage = error instanceof Error ? error.message : "读取提示词失败";
        message.error(errorMessage);
        if (errorMessage.includes("未登录") || errorMessage.includes("权限不足") || errorMessage.includes("登录状态无效")) clearSession();
    }, [categoriesQuery.error, clearSession, message, promptsQuery.error, pendingQuery.error, rejectedQuery.error]);

    const updateFilters = (next: Partial<{ keyword: string; category: string; tag: string[]; page: number; pageSize: number }>) => {
        const queryState = { keyword, category, tag, page, pageSize, ...next };
        if (next.keyword !== undefined || next.category !== undefined || next.tag !== undefined || next.pageSize !== undefined) queryState.page = 1;
        setKeyword(queryState.keyword);
        setCategory(queryState.category);
        setTag(queryState.tag);
        setPage(queryState.page);
        setPageSize(queryState.pageSize);
    };

    const currentData = activeTab === "pending" ? pendingQuery.data : activeTab === "rejected" ? rejectedQuery.data : promptsQuery.data;

    return {
        categories: categoriesQuery.data || [],
        prompts: currentData?.items || [],
        tags: activeTab === "all" ? (promptsQuery.data?.tags || []) : [],
        keyword,
        category,
        tag,
        page,
        pageSize,
        total: currentData?.total || 0,
        activeTab,
        setActiveTab,
        isLoading: categoriesQuery.isFetching || promptsQuery.isFetching || pendingQuery.isFetching || rejectedQuery.isFetching || saveMutation.isPending || deleteMutation.isPending || batchDeleteMutation.isPending,
        isSyncing: syncMutation.isPending || syncAllMutation.isPending,
        syncCategory: (category: string) => syncMutation.mutateAsync(category),
        syncAllCategories: () => syncAllMutation.mutateAsync(),
        searchPrompts: (value = keyword) => updateFilters({ keyword: value }),
        changeCategory: (value: string) => updateFilters({ category: value, tag: [] }),
        changeTag: (value: string[]) => updateFilters({ tag: value }),
        changePage: (value: number) => updateFilters({ page: value }),
        changePageSize: (value: number) => updateFilters({ pageSize: value }),
        resetFilters: () => updateFilters({ keyword: "", category: "", tag: [], page: 1, pageSize: defaultPageSize }),
        refreshPrompts: async () => {
            await categoriesQuery.refetch();
            await promptsQuery.refetch();
            await pendingQuery.refetch();
            await rejectedQuery.refetch();
        },
        savePrompt: (prompt: Partial<Prompt>) => saveMutation.mutateAsync(prompt),
        deletePrompt: (id: string) => deleteMutation.mutateAsync(id),
        deletePrompts: (ids: string[]) => batchDeleteMutation.mutateAsync(ids),
        approvePrompt: (id: string) => approveMutation.mutateAsync(id),
        rejectPrompt: (id: string) => rejectMutation.mutateAsync(id),
    };
}
