export const WELCOME_EMAIL_TEMPLATE = (firstname: string, url: string) => `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Welcome to Stellance</title>
    <link
      href="https://fonts.googleapis.com/css2?family=Inter:ital,opsz,wght@0,14..32,100..900;1,14..32,100..900&family=Montserrat:ital,wght@0,100..900;1,100..900&family=Rubik:ital,wght@0,300..900;1,300..900&display=swap"
      rel="stylesheet"
    />
  </head>

  <body
    style="
      overflow-x: hidden;
      font-family: 'Inter', sans-serif;
      margin: 0;
      padding: 0;
      background-color: #ffffff;
      box-sizing: border-box;
    "
  >
    <div
      style="
        max-width: 800px;
        margin: 0 auto;
        background: #ffffff;
        padding: 40px 12px 24px;
      "
    >
      <div style="width: 100%; margin: 0 auto">
        <div style="width: 100%; position: relative">
          <img
            src="https://res.cloudinary.com/kodenigga/image/upload/v1750181599/stellance-email-resources/banners/welcome-banner_c7jbib.png"
            alt="banner"
            style="width: 100%; height: 70%; object-fit: contain"
          />
          <div
            style="position: absolute; bottom: 60px; left: 40px; color: #fff"
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750181850/stellance-email-resources/banners/stellance_dglweo.png"
              alt="stellance logo"
              style="width: 120px; height: auto; object-fit: contain"
            />
            <p style="font-size: 28px; font-weight: 400; margin-bottom: 0px">
              Welcome to Stellance
            </p>
            <p style="font-size: 28px; font-weight: 300; margin-top: 6px">
              Verify your email to get started.
            </p>
          </div>
        </div>

        <div
          style="
            font-size: 16px;
            line-height: 28px;
            color: #1e1e1ee3;
            width: 95%;
            margin-top: 40px;
            margin-bottom: 40px;
          "
        >
          <p>Hi, ${firstname},</p>
          <p>Welcome to Stellance, your new seamless invoicing friend!</p>
          <p>
            Please verify your account below to fully activate your account.
          </p>
        </div>

        <div>
          <a
            href="${url}"
            style="
              display: block;
              width: fit-content;
              text-decoration: none;
              background: #18234f;
              color: white;
              padding: 14px 40px;
              font-weight: 600;
              font-size: 14px;
              border-radius: 24px;
              margin: 0px auto;
            "
          >
            Verify Email
          </a>
        </div>

        <div
          style="
            font-size: 16px;
            line-height: 28px;
            color: #1e1e1ee3;
            width: 95%;
            margin-top: 40px;
            padding-bottom: 40px;
          "
        >
          <p>Looking forward to supporting your business.</p>
          <div>
            <p style="margin-bottom: 0px;">Best regards,</p>
            <p style="margin-top: 0px;">The Stellance Team.</p>
          </div>
        </div>
      </div>

      <div
        style="
          font-size: 14px;
          text-align: center;
          color: #787878;
          margin-top: 36px;
          margin-bottom: 40px;
          max-width: 380px;
          margin: auto;
        "
      >
        <p>You are receiving this email because you created an account with us, please do not reply to this email. Have questions? Send an email to <a href="mailto:help@stellance.co">help@stellance.co</a></p>

        <div>
          <a
            href="https://www.facebook.com/flextables"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img src="https://res.cloudinary.com/kodenigga/image/upload/v1750183126/stellance-email-resources/socials/devicon_facebook_pxwkiv.png" alt="" />
          </a>
          <a
            href="https://www.linkedin.com/company/flextables/"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img src="https://res.cloudinary.com/kodenigga/image/upload/v1750183125/stellance-email-resources/socials/basil_linkedin-solid_qoteb5.png" alt="" />
          </a>
          <a
            href="https://x.com/useflextable"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img src="https://res.cloudinary.com/kodenigga/image/upload/v1750183127/stellance-email-resources/socials/devicon_twitter_nt7qdn.png" alt="" />
          </a>
          <a
            href="https://www.instagram.com/useflextable/"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              height: 28px;
              width: 28px;
              object-fit: contain;
            "
          >
            <img src="https://res.cloudinary.com/kodenigga/image/upload/v1750183129/stellance-email-resources/socials/skill-icons_instagram_st3b4h.png" alt="" />
          </a>
        </div>
        
        <p>Powered by Stellance</p>

      </div>
    </div>
  </body>
</html>
`;

export const RESET_PASSWORD_TEMPLATE = (
  url: string
) => `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Reset Password</title>
    <link
      href="https://fonts.googleapis.com/css2?family=Inter:ital,opsz,wght@0,14..32,100..900;1,14..32,100..900&family=Montserrat:ital,wght@0,100..900;1,100..900&family=Rubik:ital,wght@0,300..900;1,300..900&display=swap"
      rel="stylesheet"
    />
  </head>

  <body
    style="
      overflow-x: hidden;
      font-family: 'Inter', sans-serif;
      margin: 0;
      padding: 0;
      background-color: #ffffff;
      box-sizing: border-box;
    "
  >
    <div
      style="
        max-width: 800px;
        margin: 0 auto;
        background: #ffffff;
        padding: 40px 12px 24px;
      "
    >
      <div style="width: 100%; margin: 0 auto">
        <div style="width: 100%; position: relative">
          <img
            src="https://res.cloudinary.com/kodenigga/image/upload/v1750184214/stellance-email-resources/banners/lock_erg0ir.png"
            alt="banner"
            style="width: 100%; height: 70%; object-fit: contain"
          />
          <div style="position: absolute; top: 80px; left: 40px; color: #fff">
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750181850/stellance-email-resources/banners/stellance_dglweo.png"
              alt="stellance logo"
              style="width: 120px; height: auto; object-fit: contain"
            />
            <p style="font-size: 28px; font-weight: 400; margin-bottom: 0px">
              Password Reset
            </p>
          </div>
        </div>

        <div
          style="
            font-size: 16px;
            line-height: 28px;
            color: #1e1e1ee3;
            width: 95%;
            margin-top: 40px;
            margin-bottom: 40px;
          "
        >
          <p>
            We received a request to reset your password. If you didn&apos;t
            make this request, no action is needed, your account is safe and
            your password hasn&apos;t changed.
          </p>
          <p>
            If you did request a reset, please use the one time passcode below
            to complete the process or the link below:
          </p>

          <p style="font-style: italic; font-size: 14px;">This OTP is valid for a limited time and can only be used once.</p>
        </div>

        <div>
          <a
            href="${url}"
            style="
              display: block;
              width: fit-content;
              text-decoration: none;
              background: #18234f;
              color: white;
              padding: 14px 40px;
              font-weight: 600;
              font-size: 14px;
              border-radius: 24px;
              margin: 0px auto;
            "
          >
            Reset Password
          </a>

          <p style="margin: 0px auto;">${url}</p>
        </div>

        <div
          style="
            font-size: 16px;
            line-height: 28px;
            color: #1e1e1ee3;
            width: 95%;
            margin-top: 40px;
            padding-bottom: 40px;
          "
        >
          <div>
            <p style="margin-bottom: 0px">Best regards,</p>
            <p style="margin-top: 0px">The Stellance Team.</p>
          </div>
        </div>
      </div>

      <div
        style="
          font-size: 14px;
          text-align: center;
          color: #787878;
          margin-top: 36px;
          margin-bottom: 40px;
          max-width: 380px;
          margin: auto;
        "
      >
        <p>
          You are receiving this email because you created an account with us,
          please do not reply to this email. Have questions? Send an email to
          <a href="mailto:help@stellance.co">help@stellance.co</a>
        </p>

        <div>
          <a
            href="https://www.facebook.com/flextables"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183126/stellance-email-resources/socials/devicon_facebook_pxwkiv.png"
              alt=""
            />
          </a>
          <a
            href="https://www.linkedin.com/company/flextables/"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183125/stellance-email-resources/socials/basil_linkedin-solid_qoteb5.png"
              alt=""
            />
          </a>
          <a
            href="https://x.com/useflextable"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183127/stellance-email-resources/socials/devicon_twitter_nt7qdn.png"
              alt=""
            />
          </a>
          <a
            href="https://www.instagram.com/useflextable/"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              height: 28px;
              width: 28px;
              object-fit: contain;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183129/stellance-email-resources/socials/skill-icons_instagram_st3b4h.png"
              alt=""
            />
          </a>
        </div>

        <p>Powered by Stellance</p>
      </div>
    </div>
  </body>
</html>
`;

export const INVOICE_TEMPLATE = (invoiceAmount: string, invoiceCode: string, invoiceDate: string, dueDate: string, url: string) => `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Reset Password</title>
    <link
      href="https://fonts.googleapis.com/css2?family=Inter:ital,opsz,wght@0,14..32,100..900;1,14..32,100..900&family=Montserrat:ital,wght@0,100..900;1,100..900&family=Rubik:ital,wght@0,300..900;1,300..900&display=swap"
      rel="stylesheet"
    />
  </head>

  <body
    style="
      overflow-x: hidden;
      font-family: 'Inter', sans-serif;
      margin: 0;
      padding: 0;
      background-color: #ffffff;
      box-sizing: border-box;
    "
  >
    <div
      style="
        max-width: 800px;
        margin: 0 auto;
        background: #ffffff;
        padding: 40px 12px 24px;
      "
    >
      <div
        style="
          width: 70%;
          margin: 0 auto;
          background: #f0f4fc;
          padding: 20px 20px;
          margin-bottom: 20px;
        "
      >
        <div style="width: 100%">
          <div style="margin: 20px auto; text-align: center">
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750186445/stellance-email-resources/banners/blue-stellance_pmbzev.png"
              alt="stellance logo"
              style="
                width: 120px;
                height: auto;
                object-fit: contain;
                margin: 0 auto;
              "
            />
            <p style="font-weight: 700; color: #18234f">Invoice Notification</p>
          </div>

          <div
            style="
              font-size: 16px;
              line-height: 28px;
              color: #1e1e1ee3;
              width: 95%;
              margin-top: 40px;
              margin-bottom: 40px;
            "
          >
            <p>Dear Tubos Laye</p>
            <p>
              Your invoice of $3,200.00 to John Doe due on June 19, 2025 was
              successfully sent.
            </p>
            <p>Click on the button below to view the invoice.</p>
          </div>

          <div
            style="
              border: 1px solid #9FB4DD;
              border-radius: 10px;
              max-width: 310px;
              margin: auto;
            "
          >
            <div
              style="
                text-align: center;
                width: 100%;
                border-bottom: 1px solid #9FB4DD;
              "
            >
              <p style="font-weight: 700; color: #18234f">INVOICE AMOUNT</p>
              <p style="font-weight: 700; color: #18234f; margin-top: 0px">
                ${invoiceAmount}
              </p>
            </div>

            <div style="padding: 15px">
              <div style="display: flex; flex-direction: column">
                <div style="display: flex; justify-content: space-between">
                  <p style="color: #14171f; font-weight: 400; font-size: 16px">
                    Invoice No:
                  </p>
                  <p style="color: #14171f; font-weight: 600">${invoiceCode}</p>
                </div>
                <div style="display: flex; justify-content: space-between">
                  <p style="color: #14171f; font-weight: 400; font-size: 16px">
                    Invoice Date:
                  </p>
                  <p style="color: #14171f; font-weight: 600">${invoiceDate}</p>
                </div>
                <div style="display: flex; justify-content: space-between">
                  <p style="color: #14171f; font-weight: 400; font-size: 16px">
                    Due Date:
                  </p>
                  <p style="color: #14171f; font-weight: 600">${dueDate}</p>
                </div>
              </div>
              <a
                href="${url}"
                style="
                  display: block;
                  width: fit-content;
                  text-decoration: none;
                  background: #18234f;
                  color: white;
                  padding: 14px 40px;
                  font-weight: 600;
                  font-size: 14px;
                  border-radius: 24px;
                  margin: 10px auto 0px auto;
                "
              >
                View Invoice
              </a>
            </div>
          </div>

          <div
            style="
              font-size: 16px;
              line-height: 28px;
              color: #1e1e1ee3;
              width: 95%;
              margin-top: 40px;
              padding-bottom: 40px;
            "
          >
            <div>
              <p style="margin-bottom: 0px">Best regards,</p>
              <p style="margin-top: 0px">The Stellance Team.</p>
            </div>
          </div>
        </div>
      </div>

      <div
        style="
          font-size: 14px;
          text-align: center;
          color: #787878;
          margin-top: 42px;
          margin-bottom: 40px;
          max-width: 380px;
          margin: auto;
        "
      >
        <p>
          You are receiving this email because you created an account with us,
          please do not reply to this email. Have questions? Send an email to
          <a href="mailto:help@stellance.co">help@stellance.co</a>
        </p>

        <div>
          <a
            href="https://www.facebook.com/stellance"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183126/stellance-email-resources/socials/devicon_facebook_pxwkiv.png"
              alt=""
            />
          </a>
          <a
            href="https://www.linkedin.com/company/stellance/"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183125/stellance-email-resources/socials/basil_linkedin-solid_qoteb5.png"
              alt=""
            />
          </a>
          <a
            href="https://x.com/stellance"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              padding: 5px;
              height: 28px;
              width: 28px;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183127/stellance-email-resources/socials/devicon_twitter_nt7qdn.png"
              alt=""
            />
          </a>
          <a
            href="https://www.instagram.com/stellance/"
            style="
              overflow: hidden;
              display: inline-block;
              margin: 18px 10px;
              height: 28px;
              width: 28px;
              object-fit: contain;
            "
          >
            <img
              src="https://res.cloudinary.com/kodenigga/image/upload/v1750183129/stellance-email-resources/socials/skill-icons_instagram_st3b4h.png"
              alt=""
            />
          </a>
        </div>

        <p>Powered by Stellance</p>
      </div>
    </div>
  </body>
</html>`
