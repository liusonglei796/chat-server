import os

handlers = {
    'friendship_handler.go': {
        'struct': 'FriendshipHandler',
        'fields': '',
        'constructor': 'func NewFriendshipHandler() *FriendshipHandler {\n\treturn &FriendshipHandler{}\n}',
        'client': 'grpc_client.RelationClient',
        'pkg': 'relationpb'
    },
    'group_handler.go': {
        'struct': 'GroupHandler',
        'fields': '',
        'constructor': 'func NewGroupHandler() *GroupHandler {\n\treturn &GroupHandler{}\n}',
        'client': 'grpc_client.RelationClient',
        'pkg': 'relationpb'
    },
    'apply_handler.go': {
        'struct': 'ApplyHandler',
        'fields': '',
        'constructor': 'func NewApplyHandler() *ApplyHandler {\n\treturn &ApplyHandler{}\n}',
        'client': 'grpc_client.RelationClient',
        'pkg': 'relationpb'
    },
    'session_handler.go': {
        'struct': 'SessionHandler',
        'fields': '',
        'constructor': 'func NewSessionHandler() *SessionHandler {\n\treturn &SessionHandler{}\n}',
        'client': 'grpc_client.MessageClient',
        'pkg': 'messagepb'
    },
    'message_handler.go': {
        'struct': 'MessageHandler',
        'fields': '',
        'constructor': 'func NewMessageHandler() *MessageHandler {\n\treturn &MessageHandler{}\n}',
        'client': 'grpc_client.MessageClient',
        'pkg': 'messagepb'
    }
}

# we'll just manually replace the New... function and the struct fields
for f, info in handlers.items():
    path = os.path.join('internal', 'handler', f)
    if not os.path.exists(path):
        continue
    with open(path, 'r') as file:
        content = file.read()
    
    # replace struct
    import re
    # Match type XHandler struct { ... }
    struct_pattern = r'type ' + info['struct'] + r' struct \{[^\}]*\}'
    content = re.sub(struct_pattern, f'type {info["struct"]} struct {{\n}}', content)
    
    # Match func NewXHandler(...) *XHandler { ... }
    constructor_pattern = r'func New' + info['struct'] + r'\([^\)]*\)\s*\*' + info['struct'] + r'\s*\{[^\}]*\}'
    content = re.sub(constructor_pattern, info['constructor'], content)
    
    with open(path, 'w') as file:
        file.write(content)
