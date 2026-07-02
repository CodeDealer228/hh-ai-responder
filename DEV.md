```python
def parse_redirect_config(filename):
  with open(filename) as f:
    contents=f.read()
  data,_=__import__('json.decoder').JSONDecoder().raw_decode(contents[contents.find('{"redirectConfig":'):])
  return data
```

```python
>>> contents=open('resumes.html').read()
>>> contents[contents.find('{"redirectConfig":'):][:100]
'{"redirectConfig":{"strictMode":true,"topLevelDomain":"hh.ru","permittedHosts":["hh.ru","hr.zarplata'
>>> from json.decoder import JSONDecoder
>>> jd=JSONDecoder()
>>> data,_=jd.raw_decode(contents[contents.find('{"redirectConfig":'):])
>>> list(data['account'])
['firstName', 'middleName', 'lastName', 'email', 'phone']
>>> list(data)
['redirectConfig', 'topLevelSite', 'registrationSiteId', 'topLevelDomain', 'logos', 'banners', 'needShowGsuAgeRestrictionSnackbar', 'authUrl', 'applicantInfo', 'resumeLimits', 'userTargeting', 'userNotifications', 'uxfeedback', 'articleRewriteRoutes', 'supernovaUserType', 'additionalCheck', 'supernovaSearchArea', 'account', 'hhidAccount', 'headerMenu', 'defaultCountryCompanySearchId', 'globalInvitations', 'footer', 'socialNetworksLinks', 'savedAreaIds', 'userStats', 'bannersBatchUrl', 'contactPhones', 'messengers', 'renderRestriction', 'smartScript', 'stayInTouch', 'chatikFloatActivator', 'chatik', 'applicantSignup', 'anonymousUserType', 'isUseMagritteLayout', 'resumeCountriesVisibilityAgreemetsModalEnabled', 'resumeCountriesVisibilityAgreements', 'applicantUserStatuses', 'resumeAuditRecommendation', 'stateHhPro', 'applicantPaymentServices', 'applicantSkillsVerificationExpiring', 'latestResumeHash', 'addSkillToResume', 'hasAIHhPro', 'applicantResumesAiAudit', 'applicantResumesStatistics', 'applicantResumes', 'resumesExportTypes', 'allowedSMSCountries', 'applicantSuitableVacancyByResume', 'infoTip', 'authPhone', 'chatBot', 'applicantProfile', 'isAutoresponseExp', 'currencies', 'counters', 'siteCounterGroups', 'errorCode', 'session', 'displayType', 'langs', 'trl', 'userType', 'features', 'notes', 'analyticsParams', 'experiments', 'isLightPage', 'request', 'countryId', 'locale', 'hhid', 'config', 'sharedRemoteEntry', 'microFrontends', 'mainContentVisible', 'webviewAppType', 'platformInfo', 'xsrfToken', 'isCookiesPolicyInformerVisible', 'router']
>>> list(data['hhidAccount'])
['firstName', 'middleName', 'lastName', 'email', 'phone']
>>> list(data['applicantResumes'][0]['_attributes'])
['canPublishOrUpdate', 'canTouch', 'created', 'hasConditions', 'hasErrors', 'hasPublicVisibility', 'hash', 'hhid', 'id', 'isSearchable', 'lang', 'lastEditTime', 'markServiceExpireTime', 'marked', 'moderated', 'nextPublishAt', 'nextTouchAt', 'parentResumeId', 'percent', 'permission', 'publishState', 'renewal', 'renewalServiceExpireTime', 'siteId', 'sitePlatform', 'source', 'status', 'tags', 'update_timeout', 'updated', 'user', 'vacancySearchLastUsageDate', 'validation_schema']
```

```python
def find_path(data, target, current_path=None):
    if current_path is None:
        current_path = []

    if isinstance(data, dict):
        for key, value in data.items():
            path = current_path + [key]
            if value == target:
                return path
            result = find_path(value, target, path)
            if result:
                return result

    elif isinstance(data, list):
        for index, item in enumerate(data):
            path = current_path + [index]
            if item == target:
                return path
            result = find_path(item, target, path)
            if result:
                return result

    return None
>>> cfg=parse_redirect_config('resumes.html')
>>> find_path(cfg, "https://resume-profile-front.hh.ru")
['config', 'externalMicroFrontendHosts', 'resume-profile-front']
>>> cfg["config"]
{'staticHost': 'https://i.hh.ru', 'apiXhhHost': 'https://hh.ru', 'hhcdnHost': 'https://hhcdn.ru', 'imageResizingCdnHost': 'https://img.hhcdn.ru', 'devBuildNotifyEnabled': False, 'externalMicroFrontendHosts': {'applicant-services-front': 'https://applicant-services-front.hh.ru', 'employer-reviews-front': 'https://employer-reviews-front.hh.ru', 'chatik': 'https://chatik.hh.ru', 'skills-front': '', 'support-front': 'https://support-front.hh.ru', 'resume-profile-front': 'https://resume-profile-front.hh.ru', 'branding-front': 'https://branding-front.hh.ru', 'webcall-front': 'https://webcall-front.hh.ru', 'mentors-front': 'https://mentors-front.hh.ru', 'career-platform-front': 'https://career.hh.ru'}
```
