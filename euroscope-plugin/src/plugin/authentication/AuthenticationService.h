#pragma once
#include <future>
#include <optional>
#include <nlohmann/json.hpp>

#include "configuration/AppConfig.h"
#include "configuration/UserConfig.h"
#include "handlers/TimedEventHandler.h"

namespace FlightStrips::authentication {

static constexpr char base64_url_alphabet[] = {
    'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
    'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
    'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
    'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
    '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-', '_'
};

enum AuthenticationState {
    NONE = 0,
    LOGIN = 1,
    REFRESH = 2,
    AUTHENTICATED = 3
};

class AuthenticationService : public handlers::TimedEventHandler{
public:
    AuthenticationService(const std::shared_ptr<configuration::AppConfig> &appConfig, const std::shared_ptr<configuration::UserConfig> &userConfig);
    ~AuthenticationService() override;

    void Logout();
    void StartAuthentication();
    void CancelAuthentication();
    [[nodiscard]] AuthenticationState GetAuthenticationState() const;
    [[nodiscard]] std::string GetName() const;
    [[nodiscard]] std::string GetAccessToken() const;


    void OnTimer(int time) override;
    static std::optional<nlohmann::json> GetTokenPayload(const std::string &access_token);

private:
    std::shared_ptr<configuration::AppConfig> appConfig;
    std::shared_ptr<configuration::UserConfig> userConfig;
    std::atomic_int state = ATOMIC_VAR_INIT(AuthenticationState::NONE);

    std::string accessToken = "";
    std::string refreshToken = "";
    std::string name = "";
    time_t expirationTime = 0;

    void LoadFromConfig();

    bool NeedsRefresh() const;
    void StartRefresh();
    void DoRefreshFlow();
    bool ParseAndSetToken(const std::string& content);

    void DoAuthenticationFlow();
    void DoAuthenticationFlowImpl();
    bool WaitForResult(const std::future<std::optional<std::string>> &future) const;
    [[nodiscard]] std::string GetAuthorizeUrl(const std::string& code_challenge, const std::string& client_id, const std::string& redirect_uri) const;

    static void OpenBrowser(const std::string& url);


    std::thread token_thread;

    std::string GetTokenFromAuthorizationCode(const std::string& clientId, const std::string &codeVerifier, const std::string &code, const std::string &redirectUrl) const;

    static std::string generateCodeVerifier();
    static std::string base64_encode(const std::string & in);
    static std::string base64_decode(const std::string & in);
    static std::string generateCodeChallenge(const std::string& codeVerifier);
};

}
