# API Struct Classification

## User Module
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **POST** | `/login` | `LoginRequest` | `LoginRespond` |
| **POST** | `/register` | `RegisterRequest` | `RegisterRespond` |
| **POST** | `/user/smsLogin` | `SmsLoginRequest` | `LoginRespond` |
| **POST** | `/user/sendSmsCode` | `SendSmsCodeRequest` | - |
| **POST** | `/user/wsLogout` | `WsLogoutRequest` | - |
| **POST** | `/user/updateUserInfo` | `UpdateUserInfoRequest` | - |
| **GET** | `/user/getUserInfoList` | `GetUserInfoListRequest` | `[]GetUserListRespond` |
| **GET** | `/user/getUserInfo` | `GetUserInfoRequest` | `GetUserInfoRespond` |
| **POST** | `/user/ableUsers` | `AbleUsersRequest` | - |
| **POST** | `/user/disableUsers` | `AbleUsersRequest` | - |
| **POST** | `/user/deleteUsers` | `AbleUsersRequest` | - |
| **POST** | `/user/setAdmin` | `AbleUsersRequest` | - |

## Auth Module
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **POST** | `/auth/refresh` | `RefreshTokenRequest` | `map[string]string` |

## Contact Module
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **GET** | `/contact/getUserList` | `OwnlistRequest` | `[]MyUserListRespond` |
| **GET** | `/contact/loadMyJoinedGroup` | `OwnlistRequest` | `[]LoadMyJoinedGroupRespond` |
| **GET** | `/contact/getContactInfo` | `GetContactInfoRequest` | `GetContactInfoRespond` |
| **POST** | `/contact/deleteContact` | `DeleteContactRequest` | - |
| **POST** | `/contact/blackContact` | `BlackContactRequest` | - |
| **POST** | `/contact/cancelBlackContact` | `BlackContactRequest` | - |
| **POST** | `/contact/applyContact` | `ApplyContactRequest` | - |
| **GET** | `/contact/getNewContactList` | `OwnlistRequest` | `[]NewContactListRespond` |
| **POST** | `/contact/passContactApply` | `PassContactApplyRequest` | - |
| **POST** | `/contact/refuseContactApply` | `PassContactApplyRequest` | - |
| **POST** | `/contact/blackApply` | `BlackApplyRequest` | - |
| **GET** | `/contact/getAddGroupList` | `AddGroupListRequest` | `[]AddGroupListRespond` |

## Group Module
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **POST** | `/group/createGroup` | `CreateGroupRequest` | - |
| **GET** | `/group/loadMyGroup` | `OwnlistRequest` | `[]LoadMyGroupRespond` |
| **GET** | `/group/getGroupInfo` | `GetGroupInfoRequest` | `GetGroupInfoRespond` |
| **POST** | `/group/updateGroupInfo` | `UpdateGroupInfoRequest` | - |
| **POST** | `/group/dismissGroup` | `DismissGroupRequest` | - |
| **GET** | `/group/checkGroupAddMode` | `CheckGroupAddModeRequest` | `int8` |
| **POST** | `/group/enterGroupDirectly` | `EnterGroupDirectlyRequest` | - |
| **POST** | `/group/leaveGroup` | `LeaveGroupRequest` | - |
| **GET** | `/group/getGroupMemberList` | `GetGroupMemberListRequest` | `[]GetGroupMemberListRespond` |
| **POST** | `/group/removeGroupMembers` | `RemoveGroupMembersRequest` | - |
| **GET** | `/group/getGroupInfoList` | `GetGroupListRequest` | `GetGroupListWrapper` |
| **POST** | `/group/deleteGroups` | `DeleteGroupsRequest` | - |
| **POST** | `/group/setGroupsStatus` | `SetGroupsStatusRequest` | - |

## Message Module
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **GET** | `/message/getMessageList` | `GetMessageListRequest` | `[]GetMessageListRespond` |
| **GET** | `/message/getGroupMessageList` | `GetGroupMessageListRequest` | `[]GetGroupMessageListRespond` |
| **POST** | `/message/uploadAvatar` | `multipart/form-data` | `string` |
| **POST** | `/message/uploadFile` | `multipart/form-data` | `[]string` |

## Session Module
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **POST** | `/session/openSession` | `OpenSessionRequest` | `string` |
| **GET** | `/session/getUserSessionList` | `OwnlistRequest` | `[]UserSessionListRespond` |
| **GET** | `/session/getGroupSessionList` | `OwnlistRequest` | `[]GroupSessionListRespond` |
| **POST** | `/session/deleteSession` | `DeleteSessionRequest` | - |
| **GET** | `/session/checkOpenSessionAllowed` | `CreateSessionRequest` | `bool` |

## WebSocket & ChatRoom
| Method | Path | Request Struct | Response Struct |
| :--- | :--- | :--- | :--- |
| **GET** | `/wss` | Query (`client_id`) | - |
| **GET** | `/chatroom/getCurContactListInChatRoom` | `GetCurContactListInChatRoomRequest` | `[]GetCurContactListInChatRoomRespond` |
