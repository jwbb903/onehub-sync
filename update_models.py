import sqlite3
import json
import http.client
import urllib.parse
import sys

DB_PATH = '/root/onehub/one-api.db'

def get_models_from_api(base_url, key):
    """尝试从渠道 API 获取模型列表"""
    try:
        if not base_url:
            return None
        
        # 解析 URL
        parsed_url = urllib.parse.urlparse(base_url)
        host = parsed_url.netloc
        path = parsed_url.path.rstrip('/') + '/v1/models'
        
        # 建立连接
        conn = http.client.HTTPConnection(host) if parsed_url.scheme == 'http' else http.client.HTTPSConnection(host)
        
        headers = {
            'Authorization': f'Bearer {key}',
            'Content-Type': 'application/json'
        }
        
        conn.request("GET", path, headers=headers)
        response = conn.getresponse()
        
        if response.status != 200:
            print(f"  [Error] API 返回状态码: {response.status}")
            return None
            
        data = json.loads(response.read().decode())
        
        # 提取模型 ID
        models = []
        if 'data' in data and isinstance(data['data'], list):
            for item in data['data']:
                if 'id' in item:
                    models.append(item['id'])
        
        return ",".join(sorted(list(set(models)))) if models else None
    except Exception as e:
        print(f"  [Error] 请求失败: {str(e)}")
        return None

def main():
    db = sqlite3.connect(DB_PATH)
    cursor = db.cursor()
    
    # 获取所有未删除且类型为 1 (OpenAI/通用) 或 25 的渠道
    # 您可以根据需要修改这里的过滤条件
    cursor.execute("SELECT id, name, key, base_url, models FROM channels WHERE deleted_at IS NULL")
    channels = cursor.fetchall()
    
    print(f"找到 {len(channels)} 个活动渠道。开始更新...")
    
    updated_count = 0
    for cid, name, key, base_url, old_models in channels:
        print(f"正在处理渠道: {name} (ID: {cid})...")
        
        new_models = get_models_from_api(base_url, key)
        
        if new_models:
            if new_models != old_models:
                cursor.execute("UPDATE channels SET models = ? WHERE id = ?", (new_models, cid))
                print(f"  [Success] 模型已更新。")
                updated_count += 1
            else:
                print(f"  [Skip] 模型列表无变化。")
        else:
            print(f"  [Failed] 无法获取模型列表。")
            
    db.commit()
    db.close()
    print(f"\n任务完成！共更新了 {updated_count} 个渠道。")

if __name__ == "__main__":
    main()
