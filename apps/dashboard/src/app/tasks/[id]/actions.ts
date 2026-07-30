"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { cancelTask, deleteTask, retryTask } from "@/lib/queueflow";

export async function retryTaskAction(id: string) {
  await retryTask(id);
  revalidatePath("/tasks");
  revalidatePath(`/tasks/${id}`);
}

export async function cancelTaskAction(id: string) {
  await cancelTask(id);
  revalidatePath("/tasks");
  revalidatePath(`/tasks/${id}`);
}

export async function deleteTaskAction(id: string) {
  await deleteTask(id);
  revalidatePath("/tasks");
  redirect("/tasks");
}
