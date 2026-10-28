package cache

import "strconv"

const keyPrefix = "crud:v1:"

func recordKey(id int64) string {
	return keyPrefix + "record:" + strconv.FormatInt(id, 10)
}

func userRecordsKey(userID int64) string {
	return keyPrefix + "records:user:" + strconv.FormatInt(userID, 10)
}

func userKey(id int64) string {
	return keyPrefix + "user:" + strconv.FormatInt(id, 10)
}

func groupKey(id int64) string {
	return keyPrefix + "group:" + strconv.FormatInt(id, 10)
}

func groupMembersKey(id int64) string {
	return keyPrefix + "group:" + strconv.FormatInt(id, 10) + ":users"
}

func orgGroupsKey(orgID int64) string {
	return keyPrefix + "groups:org:" + strconv.FormatInt(orgID, 10)
}

func organisationKey(id int64) string {
	return keyPrefix + "org:" + strconv.FormatInt(id, 10)
}

const organisationsKey = keyPrefix + "orgs"

func orgRecordsKey(orgID int64) string {
	return keyPrefix + "records:org:" + strconv.FormatInt(orgID, 10)
}

func groupPermissionsKey(groupID int64) string {
	return keyPrefix + "perms:group:" + strconv.FormatInt(groupID, 10)
}
