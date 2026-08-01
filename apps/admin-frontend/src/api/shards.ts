import { api } from './index';

export type ShardStatus = 'active' | 'draining' | 'inactive';

export interface Shard {
  id: number;
  status: ShardStatus;
  contest_count: number;
  participant_count: number;
  orders_per_sec: number;
  created_at: string;
  updated_at: string;
}

export interface ShardDetails extends Shard {
  contests: Array<{
    id: string;
    name: string;
    participant_count: number;
    status: string;
  }>;
  memory_usage_mb: number;
  cpu_usage_percent: number;
  connection_count: number;
}

export interface ShardStats {
  shard_id: number;
  timestamp: string;
  orders_per_sec: number;
  participant_count: number;
  contest_count: number;
}

export async function getShards(): Promise<Shard[]> {
  const response = await api.get<{ shards: Shard[] }>('/api/admin/health/shards');
  return response.data.shards || [];
}

export async function getShard(shardId: number): Promise<ShardDetails> {
  const response = await api.get<ShardDetails>(`/api/admin/health/shards/${shardId}`);
  return response.data;
}

export async function drainShard(shardId: number): Promise<void> {
  await api.post(`/api/admin/health/shards/${shardId}/drain`);
}

export async function activateShard(shardId: number): Promise<void> {
  await api.post(`/api/admin/health/shards/${shardId}/activate`);
}

export async function getShardStats(): Promise<ShardStats[]> {
  const response = await api.get<{ stats: ShardStats[] }>('/api/admin/health/shards/stats');
  return response.data.stats || [];
}
