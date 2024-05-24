using Microsoft.AspNetCore.Mvc;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Controllers;

[ApiController]
[Route("/sessions")]
public class SessionController(IStripService stripService) : ControllerBase
{
    [HttpGet(Name = "GetSessions")]
    [ProducesResponseType(typeof(SessionResponseModel), StatusCodes.Status200OK)]
    public async Task<IActionResult> GetSessions()
    {
        var sessions = await stripService.GetSessionsAsync();

        var model = new SessionResponseModel
        {
            Sessions = sessions.Select(x => new SessionModel {Name = x.Session, Airport = x.Airport}).ToArray()
        };

        return Ok(model);
    }

}
