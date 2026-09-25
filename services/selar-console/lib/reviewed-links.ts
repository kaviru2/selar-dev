export type ReviewAction = "confirmed" | "relabeled" | "rejected" | "retracted" | "rolled_back";
export function reviewActions(status: string, revision: number): ReviewAction[] {
 if (status === "candidate") return ["confirmed", "relabeled", "rejected"];
 if (status === "confirmed" || status === "relabeled") return ["relabeled", "retracted", ...(revision > 0 ? ["rolled_back" as const] : [])];
 if (revision > 0) return ["rolled_back"];
 return [];
}
export function readerWitnessURL(id: string, locator?: {page?: number;block_index?: number}): string {
 const params = new URLSearchParams({docId:id});
 if (locator?.page) params.set("page",String(locator.page));
 else if (locator?.block_index !== undefined) params.set("block",String(locator.block_index));
 return `/reader?${params}`;
}
