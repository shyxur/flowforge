import { openTaskEventStream } from "@/lib/queueflow";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  try {
    const upstream = await openTaskEventStream(request.signal);
    if (!upstream.ok || !upstream.body) {
      return Response.json(
        { error: "QueueFlow event stream is unavailable." },
        { status: upstream.status || 502 },
      );
    }
    return new Response(upstream.body, {
      status: 200,
      headers: {
        "Cache-Control": "no-cache, no-transform",
        "Content-Type": "text/event-stream",
        "X-Accel-Buffering": "no",
      },
    });
  } catch {
    return Response.json(
      { error: "QueueFlow event stream is unavailable." },
      { status: 503 },
    );
  }
}
