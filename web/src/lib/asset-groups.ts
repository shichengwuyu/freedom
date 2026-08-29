import type { Asset } from "@/stores/use-asset-store";

/** 素材库二级分组：与参考图弹层「素材库」列表一致 */
export const ASSET_CATEGORY_GROUPS = ["人物", "场景", "物品", "风格", "其它"] as const;
export type AssetCategory = (typeof ASSET_CATEGORY_GROUPS)[number];

const CATEGORY_KEYWORDS: Record<Exclude<AssetCategory, "其它">, string[]> = {
    人物: ["人物", "人像", "角色", "肖像", "头像", "人物图", "portrait", "character", "person", "face"],
    场景: ["场景", "背景", "环境", "风景", "室内", "室外", "城市", "建筑", "景观", "街景", "scene", "landscape", "background", "environment"],
    物品: ["物品", "道具", "商品", "产品", "物件", "元素", "object", "product", "item", "prop"],
    风格: ["风格", "画风", "美术", "艺术", "海报", "水彩", "油画", "国风", "赛博", "动漫", "扁平", "3d", "style", "art"],
};

/** 根据资产标题 / 标签 / 来源推断归属分组，无法匹配时归为「其它」 */
export function classifyAssetCategory(asset: Asset): AssetCategory {
    const haystack = [asset.title || "", ...(asset.tags || []), asset.source || ""].join(" ").toLowerCase();
    for (const group of ["人物", "场景", "物品", "风格"] as const) {
        if (CATEGORY_KEYWORDS[group].some((keyword) => haystack.includes(keyword.toLowerCase()))) return group;
    }
    return "其它";
}

/** 将资产按人物 / 场景 / 物品 / 风格 / 其它 分组，只保留非文本资产 */
export function groupAssetsByCategory(assets: Asset[]): { name: AssetCategory; items: Asset[] }[] {
    const groups = new Map<AssetCategory, Asset[]>(ASSET_CATEGORY_GROUPS.map((name) => [name, []]));
    assets.forEach((asset) => {
        if (asset.kind === "text") return;
        groups.get(classifyAssetCategory(asset))!.push(asset);
    });
    return ASSET_CATEGORY_GROUPS.map((name) => ({ name, items: groups.get(name)! })).filter((group) => group.items.length > 0);
}
