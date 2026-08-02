import yaml

with open('docker-compose.yml', 'r') as f:
    data = yaml.safe_load(f)

for svc_name, svc in data['services'].items():
    if 'build' in svc or 'image' in svc and 'chat-server' in svc['image'] or svc_name.endswith('-service') or svc_name == 'chat-server':
        env = svc.get('environment', {})
        if isinstance(env, list):
            env.append('GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn')
        else:
            env['GOLANG_PROTOBUF_REGISTRATION_CONFLICT'] = 'warn'
        svc['environment'] = env

with open('docker-compose.yml', 'w') as f:
    yaml.dump(data, f)
