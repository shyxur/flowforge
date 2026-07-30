"use server";

import { revalidatePath } from "next/cache";
import { retryWebhookDelivery } from "@/lib/queueflow";

export async function retryWebhookDeliveryAction(id: string) {
  await retryWebhookDelivery(id);
  revalidatePath("/webhook-deliveries");
  revalidatePath(`/webhook-deliveries/${id}`);
}
